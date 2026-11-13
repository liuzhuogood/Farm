package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"farm/internal/safety"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (s *Service) registerTaskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/automation/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/automation/tasks", s.handleListTasks)
	mux.HandleFunc("GET /api/automation/tasks/{task_id}", s.handleGetTask)
	mux.HandleFunc("POST /api/automation/tasks/{task_id}/finish", s.handleFinishTask)
}
func taskSerial(r *http.Request) (string, error) {
	values, ok := r.URL.Query()["serial"]
	if !ok || len(values) != 1 || values[0] == "" || len(values[0]) > 512 || strings.ContainsAny(values[0], "\x00\r\n") {
		return "", taskError(400, "invalid_serial", "serial 必须是单个真实设备序列号")
	}
	return values[0], nil
}
func writeTaskError(w http.ResponseWriter, err error) {
	status, code := taskStatusError(err)
	writeJSON(w, status, map[string]string{"code": code, "error": err.Error(), "message": err.Error()})
}
func (s *Service) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	serial, err := taskSerial(r)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	title, code, err := parseActTitle(r.URL.RawQuery)
	if err != nil {
		writeTaskError(w, taskError(400, code, err.Error()))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil || strings.TrimSpace(string(body)) != "" {
		writeTaskError(w, taskError(400, "unexpected_body", "创建任务请求正文必须为空"))
		return
	}
	if _, err = s.resolveActDevice(serial); err != nil {
		writeTaskError(w, taskError(404, "device_unavailable", err.Error()))
		return
	}
	task, err := s.taskRuntime().create(serial, title)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}
func (s *Service) handleListTasks(w http.ResponseWriter, r *http.Request) {
	serial, err := taskSerial(r)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	tasks, err := s.tasksForDevice(r.Context(), serial)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	for i := range tasks {
		decorateTaskURLs(r, &tasks[i])
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"tasks": tasks})
}
func (s *Service) handleGetTask(w http.ResponseWriter, r *http.Request) {
	serial, err := taskSerial(r)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	task, err := s.taskForDevice(r.Context(), serial, r.PathValue("task_id"))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	decorateTaskURLs(r, &task)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"task": task})
}
func (s *Service) handleFinishTask(w http.ResponseWriter, r *http.Request) {
	serial, err := taskSerial(r)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	var input struct {
		Outcome string `json:"outcome"`
		Summary string `json:"summary"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&input); err != nil {
		writeTaskError(w, taskError(400, "invalid_request", err.Error()))
		return
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		writeTaskError(w, taskError(400, "invalid_request", "请求只能包含一个JSON对象"))
		return
	}
	if input.Outcome != "done" && input.Outcome != "failed" && input.Outcome != "stopped" {
		writeTaskError(w, taskError(400, "invalid_outcome", "outcome 必须是 done、failed 或 stopped"))
		return
	}
	if len(input.Summary) > 4096 {
		writeTaskError(w, taskError(400, "summary_too_long", "summary 最多4096字节"))
		return
	}
	task, err := s.taskRuntime().finish(r.Context(), r.PathValue("task_id"), serial, input.Outcome, input.Summary)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	decorateTaskURLs(r, &task)
	writeJSON(w, 200, map[string]any{"task": task})
}
func decorateTaskURLs(r *http.Request, t *Task) {
	convert := func(a *TaskArtifact) {
		for _, value := range []*string{&a.DownloadURL, &a.PreviewURL} {
			if strings.HasPrefix(*value, "/") {
				parsed, err := url.Parse(*value)
				if err == nil {
					*value = artifactDirectoryURL(r, parsed.Path, parsed.Query())
				}
			}
		}
	}
	for i := range t.Artifacts {
		convert(&t.Artifacts[i])
	}
	for i := range t.Steps {
		for j := range t.Steps[i].Artifacts {
			convert(&t.Steps[i].Artifacts[j])
		}
	}
}

func (s *Service) tasksForDevice(ctx context.Context, serial string) ([]Task, error) {
	rt := s.taskRuntime()
	rt.mu.Lock()
	out := make([]Task, 0)
	for _, task := range rt.tasks {
		if task.view.Serial == serial {
			out = append(out, cloneTask(task.view))
		}
	}
	history := rt.history
	rt.mu.Unlock()
	if history != nil {
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		records, err := history.ListTasks(readCtx, serial, 32)
		if err != nil {
			return nil, fmt.Errorf("读取MCP审计任务: %w", err)
		}
		for _, record := range records {
			header, steps, truncated, err := history.GetTaskDetailLimited(readCtx, record.ID, serial, 20)
			if err != nil {
				return nil, fmt.Errorf("读取MCP审计步骤: %w", err)
			}
			task, err := mapMCPTask(header, steps)
			if err != nil {
				return nil, err
			}
			task.StepsTruncated = truncated
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}
func (s *Service) taskForDevice(ctx context.Context, serial, id string) (Task, error) {
	rt := s.taskRuntime()
	rt.mu.Lock()
	if task := rt.tasks[id]; task != nil {
		if task.view.Serial != serial {
			rt.mu.Unlock()
			return Task{}, taskError(404, "task_not_found", "task_id 不属于指定设备")
		}
		out := cloneTask(task.view)
		rt.mu.Unlock()
		return out, nil
	}
	history := rt.history
	rt.mu.Unlock()
	if history != nil {
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		header, steps, truncated, err := history.GetTaskDetailLimited(readCtx, id, serial, 500)
		if err == nil {
			if header.DeviceID != serial {
				return Task{}, taskError(404, "task_not_found", "task_id 不属于指定设备")
			}
			task, mapErr := mapMCPTask(header, steps)
			task.StepsTruncated = truncated
			return task, mapErr
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Task{}, fmt.Errorf("读取MCP审计任务: %w", err)
		}
	}
	return Task{}, taskError(404, "task_not_found", "任务不存在")
}

// Existing MCP history records are read as-is; the new panel neither changes
// MCP ownership nor claims to know whether a legacy call is currently in flight.
func mapMCPTask(record safety.TaskRecord, records []safety.StepRecord) (Task, error) {
	task := Task{ID: record.ID, Serial: record.DeviceID, Title: record.Title, Source: "mcp", State: record.State, Summary: record.Summary, StartedAt: record.StartedAt, UpdatedAt: record.StartedAt, Active: false, Steps: []TaskStep{}, Artifacts: []TaskArtifact{}}
	switch record.State {
	case "running", "done", "failed", "stopped":
	default:
		return Task{}, fmt.Errorf("MCP审计任务 %s 包含未支持状态 %q", record.ID, record.State)
	}
	if !record.EndedAt.IsZero() {
		ended := record.EndedAt
		task.EndedAt = &ended
		task.UpdatedAt = ended
	}
	for _, record := range records {
		started := record.StartedAt
		ended := started.Add(time.Duration(record.ElapsedMS) * time.Millisecond)
		step := TaskStep{Index: record.Index, Title: record.Tool, Command: record.Tool + " " + record.Args, StartedAt: &started, EndedAt: &ended, ElapsedMS: record.ElapsedMS, Result: taskText(record.ScreenText), Artifacts: []TaskArtifact{}}
		switch record.Status {
		case "settled", "executed", "matched":
			step.Status = "done"
		case "approval_pending":
			step.Status = "pending"
			step.EndedAt = nil
		case "failed", "not_matched":
			step.Status = "failed"
			step.Error = record.Status
		default:
			return Task{}, fmt.Errorf("MCP审计步骤 %s/%d 包含未支持状态 %q", record.TaskID, record.Index, record.Status)
		}
		task.Steps = append(task.Steps, step)
		if ended.After(task.UpdatedAt) {
			task.UpdatedAt = ended
		}
	}
	return task, nil
}
