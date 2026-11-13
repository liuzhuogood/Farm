package safety

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryStoreFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "farm-hist-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "history.db")
	mediaDir := filepath.Join(tempDir, "media")

	store, err := OpenHistoryStore(dbPath, mediaDir)
	if err != nil {
		t.Fatalf("OpenHistoryStore 失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// 1. 创建任务
	taskID, err := store.CreateTask(ctx, "phone-test-1", "Claude-Code", "测试点餐任务")
	if err != nil {
		t.Fatalf("CreateTask 失败: %v", err)
	}

	// 2. 生成测试截图
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	shotID, err := store.SaveScreenshot(img)
	if err != nil {
		t.Fatalf("SaveScreenshot 失败: %v", err)
	}
	if shotID == "" {
		t.Fatal("期望返回 shotID")
	}

	// 3. 记录步骤
	err = store.RecordStep(ctx, StepRecord{
		TaskID:       taskID,
		Index:        1,
		Tool:         "phone_tap",
		Args:         `{"ref": 12}`,
		Intent:       "点击确认支付按钮",
		Status:       "settled",
		StartedAt:    time.Now(),
		ElapsedMS:    350,
		BeforeShotID: shotID,
		AfterShotID:  shotID,
		ScreenText:   "screen 1080x2400 · app: Dinely",
	})
	if err != nil {
		t.Fatalf("RecordStep 失败: %v", err)
	}

	// 4. 记录审批
	err = store.RecordApproval(ctx, taskID, 1, "Payments", "支付 $12.80", "allow_once", "用户同意", time.Now().Add(-2*time.Second), 2000)
	if err != nil {
		t.Fatalf("RecordApproval 失败: %v", err)
	}

	// 5. 结束任务
	err = store.FinishTask(ctx, taskID, "done", "点餐成功")
	if err != nil {
		t.Fatalf("FinishTask 失败: %v", err)
	}

	// 6. 查询任务明细
	task, steps, err := store.GetTaskDetail(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTaskDetail 失败: %v", err)
	}
	if task.StepCount != 1 {
		t.Errorf("期望步数 1，实际 %d", task.StepCount)
	}
	if task.ApprovalCount != 1 {
		t.Errorf("期望审批数 1，实际 %d", task.ApprovalCount)
	}
	if len(steps) != 1 {
		t.Fatalf("期望获取 1 个步骤，实际 %d", len(steps))
	}
	if steps[0].Tool != "phone_tap" {
		t.Errorf("步骤工具不匹配: %s", steps[0].Tool)
	}

	// 7. 导出 HTML 报告自检
	htmlStr, err := store.ExportTaskHTML(ctx, taskID)
	if err != nil {
		t.Fatalf("ExportTaskHTML 失败: %v", err)
	}
	if !strings.Contains(htmlStr, "测试点餐任务") {
		t.Errorf("导出 HTML 缺少任务标题")
	}
	if !strings.Contains(htmlStr, "data:image/jpeg;base64,") {
		t.Errorf("导出 HTML 缺少 base64 嵌入快照")
	}
}
