package automation

import (
	"context"
	"encoding/json"
	"errors"
	"farm/internal/safety"
	"log"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	taskCompletedLimit = 256
	taskOpenLimit      = 16
	taskStepLimit      = 500
	taskMetadataLimit  = 32 << 20
	taskResultLimit    = 4096
	taskStepReserve    = 64 << 10 // bounded escaped result/error and producer metadata
)

type runtimeTask struct {
	view          Task
	explicit      bool
	token         uint64
	cancel        context.CancelFunc
	done          chan struct{}
	stopRequested bool
	stopSummary   string
}
type automationTasks struct {
	mu          sync.Mutex
	tasks       map[string]*runtimeTask
	next        uint64
	persistence *automationTaskPersistence
	history     *safety.HistoryStore
	closed      bool
}
type taskRequestError struct {
	status  int
	code    string
	message string
}

func (e *taskRequestError) Error() string { return e.message }
func taskError(status int, code, message string) error {
	return &taskRequestError{status, code, message}
}

func (s *Service) taskRuntime() *automationTasks {
	s.tasksOnce.Do(func() { s.tasks = &automationTasks{tasks: map[string]*runtimeTask{}} })
	return s.tasks
}

// ConfigureTasks is called before HTTP serving. Runtime updates never wait for
// persistence; one worker coalesces snapshots, and CloseTasks bounds shutdown.
func (s *Service) ConfigureTasks(file string, history *safety.HistoryStore) error {
	rt := s.taskRuntime()
	p, loaded, err := openTaskPersistence(file, rt.snapshots)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.persistence = p
	rt.history = history
	for _, task := range loaded {
		copy := task
		rt.tasks[task.ID] = &runtimeTask{view: copy, explicit: true}
	}
	rt.trimLocked()
	rt.changedLocked()
	return nil
}
func (s *Service) CloseTasks() {
	rt := s.taskRuntime()
	rt.mu.Lock()
	rt.closed = true
	var waits []<-chan struct{}
	for _, t := range rt.tasks {
		if t.view.State != "running" {
			continue
		}
		if t.view.Active {
			t.stopRequested = true
			t.stopSummary = "Farm 服务退出"
			if t.cancel != nil {
				t.cancel()
			}
			waits = append(waits, t.done)
		} else {
			rt.finishLocked(t, "stopped", "Farm 服务退出")
		}
	}
	rt.changedLocked()
	p := rt.persistence
	rt.mu.Unlock()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for _, done := range waits {
		select {
		case <-done:
		case <-deadline.C:
			log.Printf("[AutomationTask] shutdown timeout: tasks remain in cleanup; latest running snapshot will be stopped on restart")
			if p != nil {
				p.Close()
			}
			return
		}
	}
	if p != nil {
		p.Close()
	}
}
func (rt *automationTasks) changedLocked() {
	if rt.persistence != nil {
		rt.persistence.Schedule()
	}
}
func cloneTask(in Task) Task {
	out := in
	out.Steps = append([]TaskStep{}, in.Steps...)
	out.Artifacts = append([]TaskArtifact{}, in.Artifacts...)
	for i := range out.Steps {
		out.Steps[i].Artifacts = append([]TaskArtifact{}, in.Steps[i].Artifacts...)
	}
	return out
}
func (rt *automationTasks) snapshots() []Task {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make([]Task, 0, len(rt.tasks))
	for _, t := range rt.tasks {
		out = append(out, cloneTask(t.view))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}
func taskCost(t Task) int64 {
	data, _ := json.Marshal(t)
	return int64(len(data) + 128)
}
func (rt *automationTasks) costLocked() int64 {
	var n int64
	for _, t := range rt.tasks {
		n += taskCost(t.view)
		if t.view.Active {
			for _, st := range t.view.Steps {
				if st.Status == "pending" || st.Status == "running" {
					n += st.reserve
				}
			}
		}
	}
	return n
}
func (rt *automationTasks) trimLocked() { rt.trimForLocked(0) }
func (rt *automationTasks) trimForLocked(extra int64) bool {
	var completed []*runtimeTask
	for _, t := range rt.tasks {
		if t.view.State != "running" {
			completed = append(completed, t)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].view.UpdatedAt.Before(completed[j].view.UpdatedAt) })
	totalCost := rt.costLocked()
	for len(completed) > taskCompletedLimit || totalCost+extra > taskMetadataLimit {
		if len(completed) == 0 {
			break
		}
		totalCost -= taskCost(completed[0].view)
		delete(rt.tasks, completed[0].view.ID)
		completed = completed[1:]
	}
	return totalCost+extra <= taskMetadataLimit
}
func (rt *automationTasks) makeTaskLocked(serial, title string, explicit bool) (*runtimeTask, error) {
	if rt.closed {
		return nil, taskError(503, "tasks_closed", "任务服务正在退出")
	}
	open := 0
	for _, t := range rt.tasks {
		if t.view.State != "running" {
			continue
		}
		open++
		if (t.explicit || t.view.Active) && t.view.Serial == serial {
			return nil, taskError(409, "device_task_busy", "设备已有明确任务，请指定该 task_id 或先结束任务")
		}
	}
	if open >= taskOpenLimit {
		return nil, taskError(429, "task_capacity", "最多同时保留16个进行中的任务")
	}
	now := time.Now().UTC()
	id := "task_" + strings.TrimPrefix(newActionID(), "act_")
	t := &runtimeTask{explicit: explicit, view: Task{ID: id, Serial: serial, Title: title, Source: "automation", State: "running", StartedAt: now, UpdatedAt: now, Steps: []TaskStep{}, Artifacts: []TaskArtifact{}, Directory: path.Join("tasks", id)}}
	if !rt.trimForLocked(taskCost(t.view)) {
		return nil, taskError(429, "task_budget", "任务元数据容量已满")
	}
	rt.tasks[id] = t
	rt.changedLocked()
	return t, nil
}
func (rt *automationTasks) create(serial, title string) (Task, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t, err := rt.makeTaskLocked(serial, title, true)
	if err != nil {
		return Task{}, err
	}
	return cloneTask(t.view), nil
}

type automationTaskContextKey struct{}
type automationTaskExecution struct {
	taskID string
	token  uint64
	index  int
}

func taskExecution(ctx context.Context) (automationTaskExecution, bool) {
	v, ok := ctx.Value(automationTaskContextKey{}).(automationTaskExecution)
	return v, ok
}

func (rt *automationTasks) beginBatch(id, serial, title string, steps []actStep, cancel context.CancelFunc) (string, uint64, int, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return "", 0, 0, taskError(503, "tasks_closed", "任务服务正在退出")
	}
	var t *runtimeTask
	var err error
	created := false
	if id == "" {
		t, err = rt.makeTaskLocked(serial, title, false)
		created = true
	} else {
		t = rt.tasks[id]
		if t == nil || t.view.Serial != serial {
			return "", 0, 0, taskError(404, "task_not_found", "task_id 不属于指定设备")
		}
		if t.view.Source != "automation" {
			return "", 0, 0, taskError(409, "task_source", "仅自动化任务可接收 act")
		}
		if t.view.Title != title {
			return "", 0, 0, taskError(400, "task_title_mismatch", "title 必须与 task_id 的任务标题一致")
		}
	}
	if err != nil {
		return "", 0, 0, err
	}
	if t.view.State != "running" {
		return "", 0, 0, taskError(409, "task_finished", "任务已经结束，不能追加指令")
	}
	if t.view.Active {
		return "", 0, 0, taskError(409, "task_busy", "任务已有正在执行的批次")
	}
	cleanupNew := func() {
		if created {
			delete(rt.tasks, t.view.ID)
		}
	}
	if len(t.view.Steps)+len(steps) > taskStepLimit {
		cleanupNew()
		return "", 0, 0, taskError(429, "task_step_capacity", "任务最多500步，请明确结束后新建任务")
	}
	var extra int64
	for _, step := range steps {
		extra += int64(6*len(step.command)+768) + taskStepReservation(step)
	}
	if !rt.trimForLocked(extra) {
		cleanupNew()
		return "", 0, 0, taskError(429, "task_budget", "任务元数据容量不足，指令尚未执行")
	}
	start := len(t.view.Steps) + 1
	for i, step := range steps {
		t.view.Steps = append(t.view.Steps, TaskStep{reserve: taskStepReservation(step), Index: start + i, Line: step.line, Title: step.req.Action, Command: step.command, Status: "pending", Artifacts: []TaskArtifact{}})
	}
	rt.next++
	t.token = rt.next
	t.cancel = cancel
	t.done = make(chan struct{})
	t.view.Active = true
	t.view.UpdatedAt = time.Now().UTC()
	rt.changedLocked()
	return t.view.ID, t.token, start, nil
}
func taskStepReservation(step actStep) int64 {
	n := int64(taskStepReserve)
	if step.req.Action == "screenshot" || step.req.Action == "record" {
		n += int64((8 << 10) + 48*len(step.plan.directory))
	}
	return n
}
func (rt *automationTasks) startStep(id string, token uint64, index int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t := rt.tasks[id]
	if t == nil || t.token != token || !t.view.Active || t.view.State != "running" {
		return
	}
	now := time.Now().UTC()
	st := &t.view.Steps[index-1]
	st.Status = "running"
	st.StartedAt = &now
	t.view.UpdatedAt = now
	rt.changedLocked()
}
func taskText(text string) string {
	if len(text) <= taskResultLimit {
		return text
	}
	const note = "\n[结果超过4 KiB，已截断]"
	text = text[:taskResultLimit-len(note)]
	for !utf8.ValidString(text) && len(text) > 0 {
		text = text[:len(text)-1]
	}
	return text + note
}
func taskResultText(result any) string {
	switch v := result.(type) {
	case string:
		return taskText(v)
	case actCheckResult:
		if v.Matched {
			return "命中：" + v.Source
		}
		return "未命中"
	default:
		return ""
	}
}
func (rt *automationTasks) settleStep(id string, token uint64, index int, status, result string, stepErr error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t := rt.tasks[id]
	if t == nil || t.token != token || !t.view.Active || t.view.State != "running" {
		return
	}
	st := &t.view.Steps[index-1]
	now := time.Now().UTC()
	st.Status = status
	st.EndedAt = &now
	if st.StartedAt != nil {
		st.ElapsedMS = now.Sub(*st.StartedAt).Milliseconds()
	}
	st.Result = taskText(result)
	if stepErr != nil {
		st.Error = taskText(stepErr.Error())
	}
	t.view.UpdatedAt = now
	rt.changedLocked()
}
func (rt *automationTasks) finishLocked(t *runtimeTask, outcome, summary string) {
	now := time.Now().UTC()
	t.view.State = outcome
	t.view.Summary = taskText(summary)
	t.view.EndedAt = &now
	t.view.UpdatedAt = now
	t.view.Active = false
}
func (rt *automationTasks) endBatch(id string, token uint64, batchErr error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t := rt.tasks[id]
	if t == nil || t.token != token || !t.view.Active {
		return
	}
	now := time.Now().UTC()
	for i := range t.view.Steps {
		st := &t.view.Steps[i]
		if st.Status == "running" {
			st.Status = "failed"
			st.EndedAt = &now
			if st.StartedAt != nil {
				st.ElapsedMS = now.Sub(*st.StartedAt).Milliseconds()
			}
			if batchErr != nil {
				st.Error = taskText(batchErr.Error())
			}
		} else if st.Status == "pending" {
			st.Status = "skipped"
			st.EndedAt = &now
		}
	}
	t.view.Active = false
	t.view.UpdatedAt = now
	if t.stopRequested {
		rt.finishLocked(t, "stopped", t.stopSummary)
	} else if !t.explicit {
		outcome := "done"
		summary := ""
		if batchErr != nil {
			outcome = "failed"
			summary = batchErr.Error()
		}
		rt.finishLocked(t, outcome, summary)
	}
	t.cancel = nil
	close(t.done)
	t.done = nil
	rt.trimLocked()
	rt.changedLocked()
}
func (rt *automationTasks) finish(ctx context.Context, id, serial, outcome, summary string) (Task, error) {
	rt.mu.Lock()
	t := rt.tasks[id]
	if t == nil || t.view.Serial != serial {
		rt.mu.Unlock()
		return Task{}, taskError(404, "task_not_found", "task_id 不属于指定设备")
	}
	if t.view.State != "running" {
		rt.mu.Unlock()
		return Task{}, taskError(409, "task_finished", "任务已经结束")
	}
	if t.view.Active {
		if outcome != "stopped" {
			rt.mu.Unlock()
			return Task{}, taskError(409, "task_busy", "任务正在执行，仅可请求 stopped")
		}
		t.stopRequested = true
		t.stopSummary = summary
		done := t.done
		t.cancel()
		rt.mu.Unlock()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-ctx.Done():
			return Task{}, ctx.Err()
		case <-timer.C:
			return Task{}, taskError(504, "task_stop_timeout", "已请求停止，设备操作仍在收尾，请刷新任务状态")
		}
		rt.mu.Lock()
		defer rt.mu.Unlock()
		return cloneTask(t.view), nil
	}
	rt.finishLocked(t, outcome, summary)
	rt.trimLocked()
	rt.changedLocked()
	view := cloneTask(t.view)
	rt.mu.Unlock()
	return view, nil
}

func (s *Service) recordTaskArtifact(ctx context.Context, serial, relative, kind string, size int64) {
	execution, ok := taskExecution(ctx)
	if !ok {
		return
	}
	rt := s.taskRuntime()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t := rt.tasks[execution.taskID]
	if t == nil || t.token != execution.token || !t.view.Active || t.view.State != "running" || t.view.Serial != serial || execution.index < 1 || execution.index > len(t.view.Steps) {
		return
	}
	q := url.Values{"serial": {serial}, "directory": {path.Dir(relative)}, "file": {path.Base(relative)}}
	endpoint := "/api/automation/artifacts/download?"
	artifact := TaskArtifact{ID: "artifact_" + strings.TrimPrefix(newActionID(), "act_"), Serial: serial, RelativePath: relative, Kind: kind, Size: size, DownloadURL: endpoint + q.Encode()}
	if kind == "image" || kind == "video" {
		q.Set("preview", "1")
		artifact.PreviewURL = endpoint + q.Encode()
	}
	t.view.Artifacts = append(t.view.Artifacts, artifact)
	t.view.Steps[execution.index-1].Artifacts = append(t.view.Steps[execution.index-1].Artifacts, artifact)
	t.view.Directory = path.Dir(relative)
	t.view.UpdatedAt = time.Now().UTC()
	rt.changedLocked()
}
func (s *Service) recordTaskDirectory(ctx context.Context, directory string) {
	execution, ok := taskExecution(ctx)
	if !ok {
		return
	}
	rt := s.taskRuntime()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if t := rt.tasks[execution.taskID]; t != nil && t.token == execution.token && t.view.Active && t.view.State == "running" {
		t.view.Directory = directory
		t.view.UpdatedAt = time.Now().UTC()
		rt.changedLocked()
	}
}
func (s *Service) recordTaskRemoval(ctx context.Context, relative string) {
	execution, ok := taskExecution(ctx)
	if !ok {
		return
	}
	rt := s.taskRuntime()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	t := rt.tasks[execution.taskID]
	if t == nil || t.token != execution.token || !t.view.Active || t.view.State != "running" {
		return
	}
	removed := func(a TaskArtifact) bool {
		return a.RelativePath == relative || strings.HasPrefix(a.RelativePath, relative+"/")
	}
	now := time.Now().UTC()
	for _, referenced := range rt.tasks {
		if referenced.view.Serial != t.view.Serial {
			continue
		}
		changed := false
		for i := range referenced.view.Artifacts {
			if removed(referenced.view.Artifacts[i]) && !referenced.view.Artifacts[i].Removed {
				referenced.view.Artifacts[i].Removed = true
				changed = true
			}
		}
		for i := range referenced.view.Steps {
			for j := range referenced.view.Steps[i].Artifacts {
				if removed(referenced.view.Steps[i].Artifacts[j]) && !referenced.view.Steps[i].Artifacts[j].Removed {
					referenced.view.Steps[i].Artifacts[j].Removed = true
					changed = true
				}
			}
		}
		if changed {
			referenced.view.UpdatedAt = now
		}
	}
	t.view.Directory = path.Dir(relative)
	t.view.UpdatedAt = now
	rt.changedLocked()
}

func taskStatusError(err error) (int, string) {
	var e *taskRequestError
	if errors.As(err, &e) {
		return e.status, e.code
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return 504, "task_cancelled"
	}
	return 500, "task_error"
}
