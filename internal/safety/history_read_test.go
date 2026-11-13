package safety

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestListTasksFiltersDeviceBoundsAndReportsReadFailures(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenHistoryStore(filepath.Join(dir, "history.db"), filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	first, err := store.CreateTask(ctx, "phone-a", "Agent", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateTask(ctx, "phone-a", "Agent", "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTask(ctx, "phone-b", "Agent", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE tasks SET started_at=100 WHERE id=?`, first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE tasks SET started_at=200 WHERE id=?`, second); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks(ctx, "phone-a", 1)
	if err != nil || len(tasks) != 1 || tasks[0].ID != second || tasks[0].DeviceID != "phone-a" {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	all, err := store.ListTasks(ctx, "phone-a", 256)
	if err != nil || len(all) != 2 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	for _, limit := range []int{0, 257} {
		if _, err := store.ListTasks(ctx, "phone-a", limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	if _, err := store.ListTasks(ctx, "", 1); err == nil {
		t.Fatal("accepted unscoped history lookup")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ListTasks(cancelled, "phone-a", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read err=%v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListTasks(ctx, "phone-a", 1); err == nil {
		t.Fatal("closed history read silently succeeded")
	}
}

func TestGetTaskDetailLimitedScopesDeviceAndKeepsLatestSteps(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenHistoryStore(filepath.Join(dir, "history.db"), filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	id, err := store.CreateTask(ctx, "phone-a", "Agent", "保留原任务标题")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		step := StepRecord{TaskID: id, Index: i, Tool: "phone_tap", Args: "unchanged", Intent: "原始意图", Status: "settled", StartedAt: time.Now(), ScreenText: "原始页面"}
		if i == 3 {
			step.Args = strings.Repeat("中", 5000)
			step.Intent = strings.Repeat("i", 5000)
			step.ScreenText = strings.Repeat("😀", 5000)
		}
		if err := store.RecordStep(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	task, steps, truncated, err := store.GetTaskDetailLimited(ctx, id, "phone-a", 2)
	if err != nil || !truncated || len(steps) != 2 || steps[0].Index != 3 || steps[1].Index != 4 {
		t.Fatalf("task=%+v steps=%+v truncated=%v err=%v", task, steps, truncated, err)
	}
	if task.Title != "保留原任务标题" || task.DeviceID != "phone-a" || task.StepCount != 4 {
		t.Fatalf("task header changed: %+v", task)
	}
	for _, value := range []string{steps[0].Args, steps[0].Intent, steps[0].ScreenText} {
		if !strings.Contains(value, "[内容已截断至最多 4096 字符]") || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 4200 {
			t.Fatalf("field not explicitly bounded: chars=%d", utf8.RuneCountInString(value))
		}
	}
	if !strings.HasPrefix(steps[0].Args, strings.Repeat("中", 4096)) || !strings.HasPrefix(steps[0].ScreenText, strings.Repeat("😀", 4096)) {
		t.Fatal("Unicode field prefix was not preserved")
	}
	if steps[1].Args != "unchanged" || steps[1].Intent != "原始意图" || steps[1].ScreenText != "原始页面" {
		t.Fatal("short fields changed")
	}
	_, all, truncated, err := store.GetTaskDetailLimited(ctx, id, "phone-a", 500)
	if err != nil || truncated || len(all) != 4 || all[0].Index != 1 {
		t.Fatalf("full bounded history: steps=%d truncated=%v err=%v", len(all), truncated, err)
	}
	if got, leaked, _, err := store.GetTaskDetailLimited(ctx, id, "phone-b", 2); !errors.Is(err, sql.ErrNoRows) || got.ID != "" || len(leaked) != 0 {
		t.Fatalf("cross-device task leaked: %+v %+v %v", got, leaked, err)
	}
	// The new panel reader must never rewrite the historical fields or alter
	// existing MCP exports and task counts.
	_, original, err := store.GetTaskDetail(ctx, id)
	if err != nil || utf8.RuneCountInString(original[2].Args) != 5000 || utf8.RuneCountInString(original[2].ScreenText) != 5000 {
		t.Fatalf("old history detail changed: %v", err)
	}
	for _, limit := range []int{0, 501} {
		if _, _, _, err := store.GetTaskDetailLimited(ctx, id, "phone-a", limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	if _, _, _, err := store.GetTaskDetailLimited(ctx, "", "phone-a", 2); err == nil {
		t.Fatal("accepted empty task id")
	}
	if _, _, _, err := store.GetTaskDetailLimited(ctx, id, "", 2); err == nil {
		t.Fatal("accepted empty serial")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, _, err := store.GetTaskDetailLimited(cancelled, id, "phone-a", 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read err=%v", err)
	}
}

func TestGetTaskDetailLimitedBoundsFieldsContainingNUL(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenHistoryStore(filepath.Join(dir, "history.db"), filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id, err := store.CreateTask(context.Background(), "phone", "Agent", "NUL field")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordStep(context.Background(), StepRecord{TaskID: id, Index: 1, Tool: "phone_type", Args: "prefix\x00" + strings.Repeat("x", 10000), Status: "settled", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, steps, truncated, err := store.GetTaskDetailLimited(context.Background(), id, "phone", 1)
	if err != nil || truncated || len(steps) != 1 || !strings.Contains(steps[0].Args, "[内容已截断") || len(steps[0].Args) > 4200 {
		t.Fatalf("NUL field escaped size bound: steps=%+v truncated=%v err=%v", steps, truncated, err)
	}
}
