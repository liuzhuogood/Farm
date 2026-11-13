package automation

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	debuglog "farm/internal/debug"
)

// 自动化接口日志事件类型
const (
	LogTypeRequest      = "automation.request"
	LogTypeObserve      = "automation.observe"
	LogTypeObserveStage = "automation.observe.stage"
	LogTypeDiff         = "automation.diff"
	LogTypeAction       = "automation.action"
	LogTypeResponse     = "automation.response"
	LogTypeError        = "automation.error"
)

// automationLogTag 标准输出日志前缀
const automationLogTag = "[Automation]"

// AutomationLog 自动化接口单条调用日志
type AutomationLog struct {
	At        time.Time              `json:"at"`
	Type      string                 `json:"type"`
	Title     string                 `json:"title,omitempty"`
	Endpoint  string                 `json:"endpoint,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Serial    string                 `json:"serial,omitempty"`
	Status    int                    `json:"status,omitempty"`
	Code      string                 `json:"code,omitempty"`
	ElapsedMS int64                  `json:"elapsed_ms,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// AutomationLogQuery 日志查询参数
type AutomationLogQuery struct {
	Serial string `json:"serial"`
	Type   string `json:"type,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// AutomationLogResponse 日志查询响应
type AutomationLogResponse struct {
	Serial string          `json:"serial,omitempty"`
	Count  int             `json:"count"`
	Logs   []AutomationLog `json:"logs"`
}

// logRingCapacity 内存日志环形缓冲容量
const logRingCapacity = 1024

// Task titles travel only with the request context, never through device-global
// state, so nested observation and cleanup logs retain the correct task.
type automationTitleContextKey struct{}

func (s *Service) recordAutomationLogContext(ctx context.Context, entry AutomationLog) {
	if title, ok := ctx.Value(automationTitleContextKey{}).(string); ok {
		entry.Title = title
	}
	if execution, ok := taskExecution(ctx); ok {
		if entry.Fields == nil {
			entry.Fields = map[string]interface{}{}
		}
		entry.Fields["task_id"] = execution.taskID
		if execution.index > 0 {
			entry.Fields["step_index"] = execution.index
		}
	}
	s.recordAutomationLog(entry)
}

// recordAutomationLog 记录一条自动化接口日志。
// 日志同时写入标准输出（便于进程级排查）与内存环形缓冲（便于接口自省与测试取证）。
func (s *Service) recordAutomationLog(entry AutomationLog) {
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	if entry.Fields == nil {
		entry.Fields = map[string]interface{}{}
	}

	// 标准输出：遵循项目既有 [Tag] key=value 风格
	log.Printf("%s %s serial=%s title=%q %s%s", automationLogTag, entry.Type, entry.Serial, entry.Title,
		entry.Message, formatAutomationFields(entry.Fields))

	if s.logRing != nil {
		s.logRing.Append(debuglog.Event{
			At:        entry.At,
			Type:      entry.Type,
			Title:     entry.Title,
			DeviceID:  entry.Serial,
			RequestID: entry.Code,
			Message:   entry.Message,
			Fields:    entry.Fields,
		})
	}
}

// formatAutomationFields 将附加字段格式化为 key=value 片段
func formatAutomationFields(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, fields[k]))
	}
	return " " + strings.Join(parts, " ")
}

// ListAutomationLogs 查询内存中的自动化接口日志（按时间倒序）
func (s *Service) ListAutomationLogs(query AutomationLogQuery) AutomationLogResponse {
	resp := AutomationLogResponse{Serial: query.Serial, Logs: []AutomationLog{}}
	if s.logRing == nil {
		return resp
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > logRingCapacity {
		limit = logRingCapacity
	}

	events := s.logRing.Snapshot(query.Serial, logRingCapacity)
	for _, e := range events {
		if query.Type != "" && e.Type != query.Type {
			continue
		}
		if len(resp.Logs) >= limit {
			break
		}
		resp.Logs = append(resp.Logs, AutomationLog{
			At:      e.At,
			Type:    e.Type,
			Title:   e.Title,
			Serial:  e.DeviceID,
			Code:    e.RequestID,
			Message: e.Message,
			Fields:  e.Fields,
		})
	}
	resp.Count = len(resp.Logs)
	return resp
}

// AutomationLogFields 构造自动化日志附加字段（保持字段顺序稳定）
func AutomationLogFields(pairs ...interface{}) map[string]interface{} {
	fields := make(map[string]interface{}, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			continue
		}
		fields[key] = pairs[i+1]
	}
	return fields
}

// formatAutomationNumber 将数值格式化为紧凑字符串
func formatAutomationNumber(v int) string {
	return strconv.Itoa(v)
}
