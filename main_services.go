package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"farm/internal/automation"
	"farm/internal/db"
	"farm/internal/dependencies"
	"farm/internal/inputconfig"
	"farm/internal/matchingservice"
	"farm/internal/mcp"
	"farm/internal/ocr"
	"farm/internal/safety"
	"farm/internal/serviceprefs"
	"farm/internal/servicepython"
	"farm/internal/template"
	"farm/internal/visionframes"
	"farm/internal/ws"
)

type auxiliaryServices struct {
	templates    *template.Service
	storage      *template.Storage
	ocr          *ocr.Service
	matching     *matchingservice.Manager
	preferences  *serviceprefs.Manager
	automation   *automation.Service
	visionFrames *visionframes.Store
	policy       *safety.PolicyEngine
	approvals    *safety.ApprovalCenter
	history      *safety.HistoryStore
	mcpServer    *mcp.Server
}

func configureAuxiliaryServices(dataDir string, database *db.DB, hub *ws.Hub, mux *http.ServeMux) *auxiliaryServices {
	templateStorage, err := template.NewStorage("")
	if err != nil {
		log.Printf("警告: 初始化模板存储失败: %v，尝试回退到临时目录", err)
		fallbackDir := filepath.Join(os.TempDir(), "farm-templates")
		templateStorage, err = template.NewStorage(fallbackDir)
		if err != nil {
			log.Fatalf("致命错误: 模板存储完全无法初始化: %v", err)
		}
	}
	log.Printf("模板存储目录: %s", templateStorage.BaseDir())
	templateService := template.NewService(templateStorage)
	template.RegisterRoutes(mux, templateService)
	pythonServices := servicepython.New(dataDir)
	var serviceSettings *serviceprefs.Manager
	if database != nil {
		serviceSettings = serviceprefs.New(database)
	} else {
		serviceSettings = serviceprefs.New(nil)
	}
	ocrService := ocr.NewManagedService(database, hub, pythonServices, func(enabled bool, action func() error) error {
		return serviceSettings.Control(servicepython.PaddleOCR, enabled, action)
	})
	ocrService.SetIndraControl(func(enabled bool, action func() error) error {
		return serviceSettings.Control(servicepython.IndraOCR, enabled, action)
	})
	ocrService.RegisterRoutes(mux)
	// 投屏帧缓存：浏览器/匹配服务 worker/StreamKeeper 解码后的最新画面帧，供设备总状态消费
	visionFrames := visionframes.NewStore()
	visionFrames.RegisterRoutes(mux)
	hub.InitStreamKeeper(visionFrames)

	matchingManager := matchingservice.New(matchingservice.Options{
		Python: func() (string, func(), error) { return pythonServices.Acquire(servicepython.Matching) },
		PreparePython: func(ctx context.Context, stage func(string)) (string, func(), error) {
			return pythonServices.Prepare(ctx, servicepython.Matching, stage)
		},
		Control: func(enabled bool, action func() error) error {
			return serviceSettings.Control(servicepython.Matching, enabled, action)
		},
		Devices: func() []string {
			devices := []string{}
			keeper := hub.StreamKeeper()
			for _, serial := range hub.MatchingDevices() {
				hasTemplate := false
				for _, asset := range templateStorage.ListTemplates(serial) {
					if asset.Enabled {
						hasTemplate = true
						break
					}
				}
				if hasTemplate || (keeper != nil && keeper.IsActive(serial)) {
					devices = append(devices, serial)
				}
			}
			return devices
		},
		TemplateReadable: func(id, serial string) bool {
			asset, err := templateStorage.GetTemplate(id)
			return err == nil && (asset.Serial == serial || asset.Serial == template.GlobalSerial)
		},
		API: mux, Vision: http.HandlerFunc(hub.ServeMatchingVisionWS), Frame: visionFrames.Handler(),
	})
	matchingManager.RegisterRoutes(mux)
	inputConfig := inputconfig.New(database)
	automationService := automation.NewService(hub, templateStorage, ocrService, inputConfig, dependencies.ManagedADB)
	automationService.RegisterRoutes(mux)

	// 安全审核与任务历史审计系统
	safetyDir := filepath.Join(dataDir, "data", "safety")
	historyDBPath := filepath.Join(safetyDir, "history.db")
	historyMediaDir := filepath.Join(safetyDir, "media")
	historyStore, err := safety.OpenHistoryStore(historyDBPath, historyMediaDir)
	if err != nil {
		log.Printf("警告: 初始化安全历史数据库失败: %v，将降级运行", err)
	}
	policyEngine := safety.NewEngine(safety.PresetPayments)
	approvalCenter := safety.NewApprovalCenter(60 * time.Second)
	automationService.SetSafetyProviders(policyEngine, approvalCenter)
	safety.RegisterRoutes(mux, policyEngine, approvalCenter, historyStore)

	// 原生 MCP 协议端点
	mcpServer := mcp.NewServer(automationService, policyEngine, approvalCenter, historyStore, strings.TrimSpace(string(versionBytes)))
	mux.Handle("/mcp", mcpServer.HTTPHandler())

	return &auxiliaryServices{
		templates:    templateService,
		storage:      templateStorage,
		ocr:          ocrService,
		matching:     matchingManager,
		preferences:  serviceSettings,
		automation:   automationService,
		visionFrames: visionFrames,
		policy:       policyEngine,
		approvals:    approvalCenter,
		history:      historyStore,
		mcpServer:    mcpServer,
	}
}

func (s *auxiliaryServices) stopPythonServices() error {
	var failures []error
	for _, service := range []struct {
		name string
		stop func() error
	}{
		{servicepython.Matching, s.matching.Stop},
		{servicepython.PaddleOCR, s.ocr.StopPaddle},
		{servicepython.IndraOCR, s.ocr.StopIndra},
	} {
		if err := s.preferences.StopForRuntimeChange(service.name, service.stop); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", service.name, err))
		}
	}
	return errors.Join(failures...)
}

func (s *auxiliaryServices) restore() {
	s.preferences.RestoreAfter(5*time.Second, map[string]func(context.Context) error{
		servicepython.Matching:  s.matching.Start,
		servicepython.PaddleOCR: s.ocr.StartPaddle,
		servicepython.IndraOCR:  s.ocr.StartIndra,
	})
}

func (s *auxiliaryServices) Close() {
	if s.history != nil {
		_ = s.history.Close()
	}
	s.preferences.Close()
	s.matching.Close()
	if err := s.ocr.StopPaddle(); err != nil {
		log.Printf("[OCR] 退出时停止失败: %v", err)
	}
	if err := s.ocr.StopIndra(); err != nil {
		log.Printf("[OCR] 退出时停止 Indra 失败: %v", err)
	}
	s.templates.Stop()
}
