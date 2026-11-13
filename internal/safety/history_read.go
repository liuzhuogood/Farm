package safety

import (
	"context"
	"fmt"
	"time"
)

// ListTasks reads audited MCP tasks for exactly one device. It does not alter
// task ownership or any existing MCP recording/counting behavior.
func (h *HistoryStore) ListTasks(ctx context.Context, serial string, limit int) ([]TaskRecord, error) {
	if serial == "" {
		return nil, fmt.Errorf("serial is required")
	}
	if limit < 1 || limit > 256 {
		return nil, fmt.Errorf("history task limit must be 1..256")
	}
	rows, err := h.db.QueryContext(ctx, `SELECT id, device_id, agent, title, state, summary, started_at, ended_at, step_count, approval_count
		FROM tasks WHERE device_id = ? ORDER BY started_at DESC, id DESC LIMIT ?`, serial, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]TaskRecord, 0)
	for rows.Next() {
		var task TaskRecord
		var started, ended int64
		if err := rows.Scan(&task.ID, &task.DeviceID, &task.Agent, &task.Title, &task.State, &task.Summary, &started, &ended, &task.StepCount, &task.ApprovalCount); err != nil {
			return nil, err
		}
		task.StartedAt = time.UnixMilli(started)
		if ended != 0 {
			task.EndedAt = time.UnixMilli(ended)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetTaskDetailLimited reads only a device's most recent audited steps. It is
// intentionally separate from GetTaskDetail: existing MCP exports retain their
// complete history, while the live panel has bounded rows and field sizes.
// The bool reports that earlier steps were omitted, independently of fields
// carrying their own explicit truncation marker.
func (h *HistoryStore) GetTaskDetailLimited(ctx context.Context, taskID, serial string, limit int) (TaskRecord, []StepRecord, bool, error) {
	if taskID == "" || serial == "" {
		return TaskRecord{}, nil, false, fmt.Errorf("task id and serial are required")
	}
	if limit < 1 || limit > 500 {
		return TaskRecord{}, nil, false, fmt.Errorf("history step limit must be 1..500")
	}
	var task TaskRecord
	var started, ended int64
	err := h.db.QueryRowContext(ctx, `SELECT id, device_id, agent, title, state, summary, started_at, ended_at, step_count, approval_count
		FROM tasks WHERE id = ? AND device_id = ?`, taskID, serial).Scan(&task.ID, &task.DeviceID, &task.Agent, &task.Title, &task.State, &task.Summary, &started, &ended, &task.StepCount, &task.ApprovalCount)
	if err != nil {
		return TaskRecord{}, nil, false, err
	}
	task.StartedAt = time.UnixMilli(started)
	if ended != 0 {
		task.EndedAt = time.UnixMilli(ended)
	}
	rows, err := h.db.QueryContext(ctx, `SELECT task_id, idx, tool,
		substr(args, 1, 4096) || CASE WHEN length(args) > 4096 OR instr(args, char(0)) > 0 THEN char(10) || '[内容已截断至最多 4096 字符]' ELSE '' END,
		substr(intent, 1, 4096) || CASE WHEN length(intent) > 4096 OR instr(intent, char(0)) > 0 THEN char(10) || '[内容已截断至最多 4096 字符]' ELSE '' END,
		status, started_at, elapsed_ms, before_shot, after_shot,
		substr(screen_text, 1, 4096) || CASE WHEN length(screen_text) > 4096 OR instr(screen_text, char(0)) > 0 THEN char(10) || '[内容已截断至最多 4096 字符]' ELSE '' END
		FROM steps WHERE task_id = ? ORDER BY idx DESC LIMIT ?`, taskID, limit+1)
	if err != nil {
		return task, nil, false, err
	}
	defer rows.Close()
	steps := make([]StepRecord, 0, limit+1)
	for rows.Next() {
		var step StepRecord
		var started int64
		if err := rows.Scan(&step.TaskID, &step.Index, &step.Tool, &step.Args, &step.Intent, &step.Status, &started, &step.ElapsedMS, &step.BeforeShotID, &step.AfterShotID, &step.ScreenText); err != nil {
			return task, nil, false, err
		}
		step.StartedAt = time.UnixMilli(started)
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return task, nil, false, err
	}
	truncated := len(steps) > limit
	if truncated {
		steps = steps[:limit]
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return task, steps, truncated, nil
}
