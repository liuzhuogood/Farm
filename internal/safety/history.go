package safety

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// TaskRecord 代表一个完整的 AI 执行任务
type TaskRecord struct {
	ID            string    `json:"id"`
	DeviceID      string    `json:"device_id"`
	Agent         string    `json:"agent"`
	Title         string    `json:"title"`
	State         string    `json:"state"` // running, done, failed, stopped
	Summary       string    `json:"summary,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	StepCount     int       `json:"step_count"`
	ApprovalCount int       `json:"approval_count"`
}

// StepRecord 代表任务中的单个执行步骤
type StepRecord struct {
	TaskID       string    `json:"task_id"`
	Index        int       `json:"index"`
	Tool         string    `json:"tool"`
	Args         string    `json:"args"`
	Intent       string    `json:"intent,omitempty"`
	Status       string    `json:"status"` // settled, approval_pending, failed
	StartedAt    time.Time `json:"started_at"`
	ElapsedMS    int64     `json:"elapsed_ms"`
	BeforeShotID string    `json:"before_shot_id,omitempty"`
	AfterShotID  string    `json:"after_shot_id,omitempty"`
	ScreenText   string    `json:"screen_text,omitempty"`
}

// HistoryStore 任务历史与审计流水存储
type HistoryStore struct {
	db       *sql.DB
	mediaDir string
	mu       sync.RWMutex
}

const historySchema = `
CREATE TABLE IF NOT EXISTS tasks (
	id             TEXT PRIMARY KEY,
	device_id      TEXT NOT NULL DEFAULT '',
	agent          TEXT NOT NULL DEFAULT '',
	title          TEXT NOT NULL DEFAULT '',
	state          TEXT NOT NULL DEFAULT '',
	summary        TEXT NOT NULL DEFAULT '',
	started_at     INTEGER NOT NULL,
	ended_at       INTEGER NOT NULL DEFAULT 0,
	step_count     INTEGER NOT NULL DEFAULT 0,
	approval_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tasks_started ON tasks(started_at);

CREATE TABLE IF NOT EXISTS steps (
	task_id        TEXT NOT NULL,
	idx            INTEGER NOT NULL,
	tool           TEXT NOT NULL DEFAULT '',
	args           TEXT NOT NULL DEFAULT '',
	intent         TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT '',
	started_at     INTEGER NOT NULL,
	elapsed_ms     INTEGER NOT NULL DEFAULT 0,
	before_shot    TEXT NOT NULL DEFAULT '',
	after_shot     TEXT NOT NULL DEFAULT '',
	screen_text    TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (task_id, idx)
);

CREATE TABLE IF NOT EXISTS task_approvals (
	id             TEXT PRIMARY KEY,
	task_id        TEXT NOT NULL,
	step_idx       INTEGER NOT NULL DEFAULT -1,
	rule           TEXT NOT NULL DEFAULT '',
	title          TEXT NOT NULL DEFAULT '',
	decision       TEXT NOT NULL DEFAULT '',
	note           TEXT NOT NULL DEFAULT '',
	created_at     INTEGER NOT NULL,
	decided_at     INTEGER NOT NULL DEFAULT 0,
	waited_ms      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_approvals_task ON task_approvals(task_id);
`

// OpenHistoryStore 打开或初始化历史存储库
func OpenHistoryStore(dbPath, mediaDir string) (*HistoryStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return nil, err
	}

	dsn := dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(historySchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init history schema: %w", err)
	}

	return &HistoryStore{
		db:       db,
		mediaDir: mediaDir,
	}, nil
}

// Close 关闭存储服务
func (h *HistoryStore) Close() error {
	return h.db.Close()
}

func newRandomID(prefix string) string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// CreateTask 创建新任务记录
func (h *HistoryStore) CreateTask(ctx context.Context, deviceID, agent, title string) (string, error) {
	id := newRandomID("tsk_")
	now := time.Now().UnixMilli()

	_, err := h.db.ExecContext(ctx, `INSERT INTO tasks (id, device_id, agent, title, state, started_at)
		VALUES (?, ?, ?, ?, 'running', ?)`, id, deviceID, agent, title, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// FinishTask 结束任务并写入汇总
func (h *HistoryStore) FinishTask(ctx context.Context, id, state, summary string) error {
	now := time.Now().UnixMilli()
	_, err := h.db.ExecContext(ctx, `UPDATE tasks SET state = ?, summary = ?, ended_at = ? WHERE id = ?`,
		state, summary, now, id)
	return err
}

// RecordStep 写入一条步骤记录
func (h *HistoryStore) RecordStep(ctx context.Context, step StepRecord) error {
	_, err := h.db.ExecContext(ctx, `INSERT INTO steps (task_id, idx, tool, args, intent, status, started_at, elapsed_ms, before_shot, after_shot, screen_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, idx) DO UPDATE SET tool=excluded.tool, args=excluded.args, intent=excluded.intent,
			status=excluded.status, elapsed_ms=excluded.elapsed_ms, before_shot=excluded.before_shot,
			after_shot=excluded.after_shot, screen_text=excluded.screen_text`,
		step.TaskID, step.Index, step.Tool, step.Args, step.Intent, step.Status,
		step.StartedAt.UnixMilli(), step.ElapsedMS, step.BeforeShotID, step.AfterShotID, step.ScreenText)
	if err != nil {
		return err
	}

	// 递增任务总步数
	_, _ = h.db.ExecContext(ctx, `UPDATE tasks SET step_count = step_count + 1 WHERE id = ?`, step.TaskID)
	return nil
}

// RecordApproval 关联记录一次审批动作
func (h *HistoryStore) RecordApproval(ctx context.Context, taskID string, stepIdx int, rule, title, decision, note string, created time.Time, waitedMS int64) error {
	id := newRandomID("aplog_")
	now := time.Now().UnixMilli()

	_, err := h.db.ExecContext(ctx, `INSERT INTO task_approvals (id, task_id, step_idx, rule, title, decision, note, created_at, decided_at, waited_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, stepIdx, rule, title, decision, note, created.UnixMilli(), now, waitedMS)
	if err != nil {
		return err
	}

	_, _ = h.db.ExecContext(ctx, `UPDATE tasks SET approval_count = approval_count + 1 WHERE id = ?`, taskID)
	return nil
}

// SaveScreenshot 保存快照图像到磁盘媒体目录并返回标识 ID
func (h *HistoryStore) SaveScreenshot(img image.Image) (string, error) {
	if img == nil {
		return "", nil
	}
	id := newRandomID("shot_")
	filename := id + ".jpg"
	targetPath := filepath.Join(h.mediaDir, filename)

	// 降采样至长边最多 1280px
	downscaled := downscaleImage(img, 1280)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, downscaled, &jpeg.Options{Quality: 80}); err != nil {
		return "", err
	}

	if err := os.WriteFile(targetPath, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

// downscaleImage 等比缩放图像至最长边不超过 maxSide
func downscaleImage(img image.Image, maxSide int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= maxSide && h <= maxSide) {
		return img
	}

	scale := 1.0
	if w > h {
		scale = float64(maxSide) / float64(w)
	} else {
		scale = float64(maxSide) / float64(h)
	}

	nw := int(float64(w) * scale)
	nh := int(float64(h) * scale)
	if nw <= 0 {
		nw = 1
	}
	if nh <= 0 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		srcY := b.Min.Y + int(float64(y)/scale)
		if srcY >= b.Max.Y {
			srcY = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			srcX := b.Min.X + int(float64(x)/scale)
			if srcX >= b.Max.X {
				srcX = b.Max.X - 1
			}
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
	return dst
}

// ReadScreenshotBytes 读取截图原始数据
func (h *HistoryStore) ReadScreenshotBytes(id string) ([]byte, error) {
	if id == "" {
		return nil, nil
	}
	path := filepath.Join(h.mediaDir, id+".jpg")
	return os.ReadFile(path)
}

// GetTaskDetail 获取单个任务及其步骤明细
func (h *HistoryStore) GetTaskDetail(ctx context.Context, taskID string) (TaskRecord, []StepRecord, error) {
	var t TaskRecord
	var started, ended int64

	err := h.db.QueryRowContext(ctx, `SELECT id, device_id, agent, title, state, summary, started_at, ended_at, step_count, approval_count
		FROM tasks WHERE id = ?`, taskID).Scan(&t.ID, &t.DeviceID, &t.Agent, &t.Title, &t.State, &t.Summary,
		&started, &ended, &t.StepCount, &t.ApprovalCount)
	if err != nil {
		return TaskRecord{}, nil, err
	}
	t.StartedAt = time.UnixMilli(started)
	if ended > 0 {
		t.EndedAt = time.UnixMilli(ended)
	}

	rows, err := h.db.QueryContext(ctx, `SELECT task_id, idx, tool, args, intent, status, started_at, elapsed_ms, before_shot, after_shot, screen_text
		FROM steps WHERE task_id = ? ORDER BY idx ASC`, taskID)
	if err != nil {
		return t, nil, err
	}
	defer rows.Close()

	var steps []StepRecord
	for rows.Next() {
		var s StepRecord
		var stStarted int64
		if err := rows.Scan(&s.TaskID, &s.Index, &s.Tool, &s.Args, &s.Intent, &s.Status, &stStarted, &s.ElapsedMS,
			&s.BeforeShotID, &s.AfterShotID, &s.ScreenText); err != nil {
			return t, nil, err
		}
		s.StartedAt = time.UnixMilli(stStarted)
		steps = append(steps, s)
	}

	return t, steps, nil
}

// ExportTaskHTML 导出单任务自包含完整 HTML 审计报告
func (h *HistoryStore) ExportTaskHTML(ctx context.Context, taskID string) (string, error) {
	task, steps, err := h.GetTaskDetail(ctx, taskID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	fmt.Fprintf(&sb, "<title>Farm 任务审计报告 · %s</title>", html.EscapeString(task.Title))
	sb.WriteString(`
<style>
  :root { --bg: #0f1115; --card: #181b22; --border: #2a2f3a; --fg: #e6e9ef; --muted: #949cad; --accent: #4c8dff; --ok: #3fb950; --err: #f85149; }
  body { margin: 0; padding: 24px 16px 80px; background: var(--bg); color: var(--fg); font: 14px/1.6 system-ui, sans-serif; max-width: 960px; margin: 0 auto; }
  h1 { font-size: 22px; margin: 0 0 6px; }
  .meta { color: var(--muted); font-size: 13px; margin-bottom: 24px; padding-bottom: 14px; border-bottom: 1px solid var(--border); }
  .card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 16px; margin-bottom: 16px; }
  .step-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; font-weight: 600; }
  .badge { font-size: 11px; padding: 2px 6px; border-radius: 4px; background: rgba(76,141,255,.15); color: var(--accent); }
  .badge.ok { background: rgba(63,185,80,.15); color: var(--ok); }
  .shots { display: flex; gap: 12px; margin: 12px 0 6px; flex-wrap: wrap; }
  .shot-wrap { flex: 1 1 200px; min-width: 160px; text-align: center; }
  .shot-wrap img { width: 100%; max-height: 380px; object-fit: contain; background: #000; border-radius: 6px; border: 1px solid var(--border); }
  .shot-label { font-size: 12px; color: var(--muted); margin-top: 4px; }
  pre { background: #090a0d; padding: 10px; border-radius: 6px; overflow-x: auto; font-family: ui-monospace, monospace; font-size: 12px; }
</style>
</head><body>
`)
	fmt.Fprintf(&sb, "<h1>任务审计报告：%s</h1>", html.EscapeString(task.Title))
	fmt.Fprintf(&sb, "<div class=\"meta\">任务ID: <code>%s</code> · 设备: <code>%s</code> · 执行Agent: <code>%s</code> · 状态: <strong>%s</strong> · 步骤数: %d</div>",
		task.ID, task.DeviceID, task.Agent, task.State, task.StepCount)

	if task.Summary != "" {
		fmt.Fprintf(&sb, "<div class=\"card\"><strong>任务完成摘要：</strong><p>%s</p></div>", html.EscapeString(task.Summary))
	}

	sb.WriteString("<h2>执行时间线与步骤明细</h2>")
	for _, s := range steps {
		sb.WriteString("<div class=\"card\">")
		fmt.Fprintf(&sb, "<div class=\"step-head\"><span>步骤 #%d: <code class=\"badge\">%s</code></span><span class=\"badge ok\">耗时 %d ms</span></div>",
			s.Index, html.EscapeString(s.Tool), s.ElapsedMS)
		if s.Intent != "" {
			fmt.Fprintf(&sb, "<div><strong>意图说明:</strong> %s</div>", html.EscapeString(s.Intent))
		}
		if s.Args != "" {
			fmt.Fprintf(&sb, "<div style=\"margin-top:6px\"><strong>调用参数:</strong> <code>%s</code></div>", html.EscapeString(s.Args))
		}

		// 嵌入前后截图
		var b64Before, b64After string
		if s.BeforeShotID != "" {
			if raw, _ := h.ReadScreenshotBytes(s.BeforeShotID); len(raw) > 0 {
				b64Before = base64.StdEncoding.EncodeToString(raw)
			}
		}
		if s.AfterShotID != "" {
			if raw, _ := h.ReadScreenshotBytes(s.AfterShotID); len(raw) > 0 {
				b64After = base64.StdEncoding.EncodeToString(raw)
			}
		}

		if b64Before != "" || b64After != "" {
			sb.WriteString("<div class=\"shots\">")
			if b64Before != "" {
				fmt.Fprintf(&sb, "<div class=\"shot-wrap\"><img src=\"data:image/jpeg;base64,%s\"><div class=\"shot-label\">操作前快照</div></div>", b64Before)
			}
			if b64After != "" {
				fmt.Fprintf(&sb, "<div class=\"shot-wrap\"><img src=\"data:image/jpeg;base64,%s\"><div class=\"shot-label\">操作后快照</div></div>", b64After)
			}
			sb.WriteString("</div>")
		}

		if s.ScreenText != "" {
			fmt.Fprintf(&sb, "<details><summary style=\"cursor:pointer;color:var(--accent);margin-top:8px\">查看该步屏幕感知文本</summary><pre>%s</pre></details>",
				html.EscapeString(s.ScreenText))
		}

		sb.WriteString("</div>")
	}

	sb.WriteString("</body></html>")
	return sb.String(), nil
}
