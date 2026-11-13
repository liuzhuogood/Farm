package automation

import (
	"context"
	"encoding/json"
	"farm/internal/device"
	"farm/internal/safety"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func taskCall(svc *Service, method, endpoint, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, endpoint, strings.NewReader(body)))
	return w
}
func createPanelTask(t *testing.T, svc *Service, serial, title string) Task {
	t.Helper()
	w := taskCall(svc, "POST", "/api/automation/tasks?"+url.Values{"serial": {serial}, "title": {title}}.Encode(), "")
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Task Task `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Task
}
func taskAct(svc *Service, id, serial, title, body string) *httptest.ResponseRecorder {
	q := url.Values{"serial": {serial}, "title": {title}}
	if id != "" {
		q.Set("task_id", id)
	}
	return taskCall(svc, "POST", "/api/automation/act?"+q.Encode(), body)
}
func getPanelTask(t *testing.T, svc *Service, id, serial string) Task {
	t.Helper()
	w := taskCall(svc, "GET", "/api/automation/tasks/"+id+"?"+url.Values{"serial": {serial}}.Encode(), "")
	if w.Code != 200 {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Task Task `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Task
}

func TestTaskExplicitLifecycleSpansBatchesAndRequiresIdentity(t *testing.T) {
	svc, _ := newActTestService()
	task := createPanelTask(t, svc, "phone", "检查打卡")
	if task.Active || task.State != "running" || task.Source != "automation" || task.Directory != "tasks/"+task.ID {
		t.Fatalf("new task=%+v", task)
	}
	for _, body := range []string{"sleep 0", "sleep 0\nsleep 0"} {
		w := taskAct(svc, task.ID, "phone", task.Title, body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), task.ID) {
			t.Fatalf("act=%d %s", w.Code, w.Body.String())
		}
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if task.State != "running" || task.Active || len(task.Steps) != 3 {
		t.Fatalf("multi-batch task=%+v", task)
	}
	for i, step := range task.Steps {
		if step.Index != i+1 || step.Status != "done" || step.Command != "sleep 0" {
			t.Fatalf("step=%+v", step)
		}
	}
	if w := taskAct(svc, "", "phone", task.Title, "sleep 0"); w.Code != 409 {
		t.Fatalf("legacy act bypassed lease: %d %s", w.Code, w.Body.String())
	}
	if w := taskAct(svc, task.ID, "phone", "different", "sleep 0"); w.Code != 400 {
		t.Fatal("title mismatch accepted", w.Code)
	}
	if w := taskCall(svc, "GET", "/api/automation/tasks/"+task.ID+"?serial=other", ""); w.Code != 404 {
		t.Fatal("cross-device task exposed", w.Code)
	}
	w := taskCall(svc, "POST", "/api/automation/tasks/"+task.ID+"/finish?serial=phone", `{"outcome":"done","summary":"已检查"}`)
	if w.Code != 200 {
		t.Fatalf("finish=%d %s", w.Code, w.Body.String())
	}
	if w = taskAct(svc, task.ID, "phone", task.Title, "sleep 0"); w.Code != 409 {
		t.Fatal("ended task accepted old request", w.Code)
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if task.State != "done" || task.EndedAt == nil || task.Summary != "已检查" {
		t.Fatalf("finished=%+v", task)
	}
	logs := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone"})
	found := false
	for _, entry := range logs.Logs {
		if entry.Fields["task_id"] == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("task ID missing from action logs")
	}
}

func TestTaskFailureKeepsExplicitTaskRecoverableAndSkipsRemaining(t *testing.T) {
	svc, _ := newActTestService()
	svc.actTransport = nil
	task := createPanelTask(t, svc, "phone", "recover")
	w := taskAct(svc, task.ID, "phone", task.Title, "keyevent back\nsleep 0")
	if w.Code != 502 {
		t.Fatalf("failed batch %d %s", w.Code, w.Body.String())
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if task.State != "running" || task.Active || task.Steps[0].Status != "failed" || task.Steps[1].Status != "skipped" || task.Steps[0].Error == "" {
		t.Fatalf("failed task=%+v", task)
	}
	if w := taskAct(svc, task.ID, "phone", task.Title, "sleep 0"); w.Code != 200 {
		t.Fatalf("recovery=%d %s", w.Code, w.Body.String())
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if len(task.Steps) != 3 || task.Steps[2].Status != "done" {
		t.Fatalf("recovered=%+v", task)
	}
}

func TestTaskStopCancelsRunningBatchAndPreventsStaleOverwrite(t *testing.T) {
	svc, _ := newActTestService()
	task := createPanelTask(t, svc, "phone", "stop")
	done := make(chan *httptest.ResponseRecorder, 1)
	id, title := task.ID, task.Title
	go func() { done <- taskAct(svc, id, "phone", title, "sleep 30000\nsleep 0") }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		task = getPanelTask(t, svc, task.ID, "phone")
		if task.Active && len(task.Steps) > 0 && task.Steps[0].Status == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("batch never entered running state")
		}
		time.Sleep(time.Millisecond)
	}
	if w := taskCall(svc, "POST", "/api/automation/tasks/"+task.ID+"/finish?serial=phone", `{"outcome":"done","summary":""}`); w.Code != 409 {
		t.Fatal("finish done interrupted active batch", w.Code)
	}
	w := taskCall(svc, "POST", "/api/automation/tasks/"+task.ID+"/finish?serial=phone", `{"outcome":"stopped","summary":"用户停止"}`)
	if w.Code != 200 {
		t.Fatalf("stop=%d %s", w.Code, w.Body.String())
	}
	select {
	case result := <-done:
		if result.Code != 504 {
			t.Fatalf("cancelled act code=%d", result.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("stopped batch did not return")
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if task.State != "stopped" || task.Active || task.Steps[0].Status != "failed" || task.Steps[1].Status != "skipped" {
		t.Fatalf("stopped task=%+v", task)
	}
	if w := taskAct(svc, task.ID, "phone", task.Title, "sleep 0"); w.Code != 409 {
		t.Fatal("stopped task accepted stale continuation", w.Code)
	}
}

func TestTaskLegacyBatchesAreDistinctAndDoNotMergeTitles(t *testing.T) {
	svc, _ := newActTestService()
	for i := 0; i < 2; i++ {
		if w := taskAct(svc, "", "phone", "same-title", "sleep 0"); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	tasks, err := svc.tasksForDevice(context.Background(), "phone")
	if err != nil || len(tasks) != 2 || tasks[0].ID == tasks[1].ID {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	for _, task := range tasks {
		if task.State != "done" || task.Active || len(task.Steps) != 1 {
			t.Fatalf("batch=%+v", task)
		}
	}
}

func TestTaskArtifactsUseProducerMetadataAndRemovalMarksOnlyOwnFiles(t *testing.T) {
	svc, _ := artifactTestService(t)
	svc.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "phone", State: "device"}} })
	task := createPanelTask(t, svc, "phone", "产物")
	if w := taskAct(svc, task.ID, "phone", task.Title, "mkdir results\nscreenshot results"); w.Code != 200 {
		t.Fatalf("artifact act=%d %s", w.Code, w.Body.String())
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if len(task.Artifacts) != 1 || task.Artifacts[0].Kind != "image" || task.Artifacts[0].Size <= 0 || task.Artifacts[0].Serial != "phone" || task.Artifacts[0].PreviewURL == "" || len(task.Steps[1].Artifacts) != 1 {
		t.Fatalf("producer artifact=%+v", task)
	}
	artifact := task.Artifacts[0]
	if w := taskAct(svc, task.ID, "phone", task.Title, "rm "+artifact.RelativePath); w.Code != 200 {
		t.Fatalf("rm=%d %s", w.Code, w.Body.String())
	}
	task = getPanelTask(t, svc, task.ID, "phone")
	if !task.Artifacts[0].Removed || !task.Steps[1].Artifacts[0].Removed {
		t.Fatal("removed artifact not recorded")
	}
}

func TestTaskCapacityRejectsBeforeDeviceActionAndKeepsOpenTasks(t *testing.T) {
	svc, transport := newActTestService()
	task := createPanelTask(t, svc, "phone", "capacity")
	rt := svc.taskRuntime()
	rt.mu.Lock()
	for i := 0; i < 500; i++ {
		rt.tasks[task.ID].view.Steps = append(rt.tasks[task.ID].view.Steps, TaskStep{Index: i + 1, Status: "done", Artifacts: []TaskArtifact{}})
	}
	rt.mu.Unlock()
	if w := taskAct(svc, task.ID, "phone", task.Title, "keyevent back"); w.Code != 429 {
		t.Fatalf("step capacity %d %s", w.Code, w.Body.String())
	}
	if len(transport.requests) != 0 {
		t.Fatal("overflow touched device")
	}
	if got := getPanelTask(t, svc, task.ID, "phone"); got.State != "running" || len(got.Steps) != 500 {
		t.Fatal("capacity handling dropped active task")
	}
	if got := taskText(strings.Repeat("中文", 3000)); len(got) > 4096 || !strings.Contains(got, "已截断") {
		t.Fatal("result budget/truncation missing")
	}
}

func TestTaskRuntimePersistsAndShowsMCPHistoryWithoutClaimingInFlight(t *testing.T) {
	dir := t.TempDir()
	history, err := safety.OpenHistoryStore(filepath.Join(dir, "history.db"), filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()
	id, err := history.CreateTask(context.Background(), "phone", "Agent", "old MCP")
	if err != nil {
		t.Fatal(err)
	}
	if err = history.RecordStep(context.Background(), safety.StepRecord{TaskID: id, Index: 1, Tool: "phone_key", Args: "key=home", Status: "settled", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "tasks.json")
	svc, _ := newActTestService()
	if err = svc.ConfigureTasks(file, history); err != nil {
		t.Fatal(err)
	}
	task := createPanelTask(t, svc, "phone", "explicit")
	if w := taskAct(svc, task.ID, "phone", task.Title, "sleep 0"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	svc.CloseTasks()
	restarted, _ := newActTestService()
	if err = restarted.ConfigureTasks(file, history); err != nil {
		t.Fatal(err)
	}
	defer restarted.CloseTasks()
	got := getPanelTask(t, restarted, task.ID, "phone")
	if got.State != "stopped" || got.Active || len(got.Steps) != 1 {
		t.Fatalf("restart=%+v", got)
	}
	mcp := getPanelTask(t, restarted, id, "phone")
	if mcp.Source != "mcp" || mcp.Active || mcp.State != "running" || len(mcp.Steps) != 1 || mcp.Steps[0].Status != "done" {
		t.Fatalf("MCP mapping=%+v", mcp)
	}
	if w := taskCall(restarted, "POST", "/api/automation/tasks/"+id+"/finish?serial=phone", `{"outcome":"stopped","summary":""}`); w.Code != 404 {
		t.Fatal("new runtime altered legacy MCP ownership", w.Code)
	}
}

func TestTaskCreateAndFinishValidateExplicitFields(t *testing.T) {
	svc, _ := newActTestService()
	for _, endpoint := range []string{"/api/automation/tasks?serial=phone", "/api/automation/tasks?serial=phone&title=a&title=b", "/api/automation/tasks?serial=phone&title=a&serial=other"} {
		if w := taskCall(svc, "POST", endpoint, ""); w.Code != 400 {
			t.Fatalf("accepted %s => %d", endpoint, w.Code)
		}
	}
	task := createPanelTask(t, svc, "phone", "validation")
	for _, body := range []string{`{"outcome":"partial","summary":""}`, `{"outcome":"done","summary":"","unexpected":1}`, `{"outcome":"done","summary":""}{}`} {
		if w := taskCall(svc, "POST", fmt.Sprintf("/api/automation/tasks/%s/finish?serial=phone", task.ID), body); w.Code != 400 {
			t.Fatalf("accepted finish %s =>%d", body, w.Code)
		}
	}
}

func TestTaskOpenCapacityAndCompletedEvictionPreserveOpenWork(t *testing.T) {
	svc, _ := newActTestService()
	rt := svc.taskRuntime()
	open := make([]Task, 0, 16)
	for i := 0; i < 16; i++ {
		task, err := rt.create(fmt.Sprintf("phone-%d", i), "open")
		if err != nil {
			t.Fatal(err)
		}
		open = append(open, task)
	}
	if _, err := rt.create("overflow", "extra"); err == nil {
		t.Fatal("17th open task accepted")
	}
	if _, err := rt.finish(context.Background(), open[0].ID, open[0].Serial, "done", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 270; i++ {
		task, err := rt.create("history", "finished")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rt.finish(context.Background(), task.ID, task.Serial, "done", ""); err != nil {
			t.Fatal(err)
		}
	}
	tasks := rt.snapshots()
	completed, active := 0, 0
	for _, task := range tasks {
		if task.State == "running" {
			active++
		} else {
			completed++
		}
	}
	if completed != 256 || active != 15 {
		t.Fatalf("capacity completed=%d running=%d", completed, active)
	}
	for _, task := range open[1:] {
		if got, err := svc.taskForDevice(context.Background(), task.Serial, task.ID); err != nil || got.State != "running" {
			t.Fatalf("trim discarded open task %s: %v", task.ID, err)
		}
	}
	escaped := Task{Steps: []TaskStep{{Command: strings.Repeat("\x01", 8192)}}}
	if taskCost(escaped) < 6*8192 {
		t.Fatal("metadata budget ignored JSON escaping")
	}
}

func TestTaskConcurrentDevicesKeepStepsAndTitlesIsolated(t *testing.T) {
	svc, _ := newActTestService()
	svc.SetDeviceProvider(func() []device.Device {
		return []device.Device{{Serial: "one", State: "device"}, {Serial: "two", State: "device"}}
	})
	one := createPanelTask(t, svc, "one", "同名任务")
	two := createPanelTask(t, svc, "two", "同名任务")
	done := make(chan *httptest.ResponseRecorder, 2)
	for _, task := range []Task{one, two} {
		go func(task Task) { done <- taskAct(svc, task.ID, task.Serial, task.Title, "sleep 30\nsleep 0") }(task)
	}
	for i := 0; i < 2; i++ {
		select {
		case w := <-done:
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("independent device tasks blocked")
		}
	}
	for _, expected := range []Task{one, two} {
		task := getPanelTask(t, svc, expected.ID, expected.Serial)
		if len(task.Steps) != 2 || task.Active || task.Serial != expected.Serial || task.State != "running" {
			t.Fatalf("device state crossed: %+v", task)
		}
	}
	if one.ID == two.ID {
		t.Fatal("task identity reused across devices")
	}
}

func TestTaskRemovalUpdatesSameDeviceHistoryWithoutChangingOtherDirectories(t *testing.T) {
	svc, _ := newActTestService()
	rt := svc.taskRuntime()
	previous, err := rt.create("phone", "previous")
	if err != nil {
		t.Fatal(err)
	}
	artifact := TaskArtifact{ID: "old-image", Serial: "phone", RelativePath: "exports/old.png", Kind: "image", DownloadURL: "/download"}
	rt.mu.Lock()
	rt.tasks[previous.ID].view.Directory = "exports"
	rt.tasks[previous.ID].view.Artifacts = []TaskArtifact{artifact}
	rt.tasks[previous.ID].view.Steps = []TaskStep{{Index: 1, Status: "done", Artifacts: []TaskArtifact{artifact}}}
	rt.mu.Unlock()
	if _, err = rt.finish(context.Background(), previous.ID, "phone", "done", ""); err != nil {
		t.Fatal(err)
	}
	other, err := rt.create("other", "same path other device")
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	different := artifact
	different.Serial = "other"
	rt.tasks[other.ID].view.Directory = "other-original"
	rt.tasks[other.ID].view.Artifacts = []TaskArtifact{different}
	rt.mu.Unlock()
	current, err := rt.create("phone", "cleanup")
	if err != nil {
		t.Fatal(err)
	}
	steps, _, err := parseActBatch("rm exports", "phone")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, token, index, err := rt.beginBatch(current.ID, "phone", current.Title, steps, cancel)
	if err != nil {
		t.Fatal(err)
	}
	svc.recordTaskRemoval(context.WithValue(ctx, automationTaskContextKey{}, automationTaskExecution{taskID: id, token: token, index: index}), "exports")
	rt.endBatch(id, token, nil)
	old, err := svc.taskForDevice(context.Background(), "phone", previous.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !old.Artifacts[0].Removed || !old.Steps[0].Artifacts[0].Removed || old.Directory != "exports" {
		t.Fatalf("historical reference not marked or directory changed: %+v", old)
	}
	untouched, err := svc.taskForDevice(context.Background(), "other", other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Artifacts[0].Removed || untouched.Directory != "other-original" {
		t.Fatalf("another device metadata changed: %+v", untouched)
	}
	cleaner, err := svc.taskForDevice(context.Background(), "phone", current.ID)
	if err != nil || cleaner.Directory != "." {
		t.Fatalf("cleanup parent directory=%q err=%v", cleaner.Directory, err)
	}
}
