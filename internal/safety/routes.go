package safety

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RegisterRoutes 在 mux 上注册所有安全审核与审计相关的 HTTP 端点
func RegisterRoutes(mux *http.ServeMux, policy *PolicyEngine, approvals *ApprovalCenter, history *HistoryStore) {
	// 1. 待办审批相关端点
	mux.HandleFunc("GET /api/safety/approvals", func(w http.ResponseWriter, r *http.Request) {
		list := approvals.List(false)
		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(list),
			"items": list,
		})
	})

	mux.HandleFunc("POST /api/safety/approvals/", func(w http.ResponseWriter, r *http.Request) {
		// 路由: POST /api/safety/approvals/{id}/decide
		path := r.URL.Path
		if !strings.HasSuffix(path, "/decide") {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 5 {
			writeError(w, http.StatusBadRequest, "invalid_path", "路径格式错误")
			return
		}
		approvalID := parts[3]

		var body struct {
			Decision UserDecision `json:"decision"`
			Note     string       `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		switch body.Decision {
		case DecisionAllowOnce, DecisionAllowTask, DecisionAllowAlways, DecisionDeny, DecisionTakeOver:
		default:
			writeError(w, http.StatusBadRequest, "invalid_decision", "无效的审核决定")
			return
		}
		err := approvals.Decide(approvalID, body.Decision, body.Note)
		if err != nil {
			writeError(w, http.StatusBadRequest, "decide_failed", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"approval_id": approvalID,
			"decision":    body.Decision,
		})
	})

	// 2. 安全策略配置端点
	mux.HandleFunc("GET /api/safety/policy", func(w http.ResponseWriter, r *http.Request) {
		policy.mu.RLock()
		defer policy.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"preset": policy.preset.String(), "preset_id": int(policy.preset),
			"rules_count": len(policy.customRules), "always_allows": policy.alwaysAllow,
		})
	})

	mux.HandleFunc("PUT /api/safety/policy", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Preset int `json:"preset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		if body.Preset < int(PresetNone) || body.Preset > int(PresetEvery) {
			writeError(w, http.StatusBadRequest, "invalid_preset", "无效的审核策略")
			return
		}
		policy.SetPreset(Preset(body.Preset))
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"preset": policy.Preset().String(),
		})
	})

	// 3. 任务历史与审计流水
	mux.HandleFunc("GET /api/safety/history/tasks", func(w http.ResponseWriter, r *http.Request) {
		if history == nil {
			writeError(w, http.StatusServiceUnavailable, "no_history", "历史服务未初始化")
			return
		}

		limit := 50
		if qLimit := r.URL.Query().Get("limit"); qLimit != "" {
			if v, err := strconv.Atoi(qLimit); err == nil && v > 0 {
				limit = v
			}
		}

		// 从 sqlite 查询任务
		rows, err := history.db.QueryContext(r.Context(),
			`SELECT id, device_id, agent, title, state, summary, started_at, ended_at, step_count, approval_count
			 FROM tasks ORDER BY started_at DESC LIMIT ?`, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
			return
		}
		defer rows.Close()

		var tasks []TaskRecord
		for rows.Next() {
			var t TaskRecord
			var started, ended int64
			if err := rows.Scan(&t.ID, &t.DeviceID, &t.Agent, &t.Title, &t.State, &t.Summary,
				&started, &ended, &t.StepCount, &t.ApprovalCount); err == nil {
				t.StartedAt = timeFromMS(started)
				if ended > 0 {
					t.EndedAt = timeFromMS(ended)
				}
				tasks = append(tasks, t)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(tasks),
			"tasks": tasks,
		})
	})

	mux.HandleFunc("GET /api/safety/history/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if history == nil {
			writeError(w, http.StatusServiceUnavailable, "no_history", "历史服务未初始化")
			return
		}

		path := r.URL.Path
		isExport := strings.HasSuffix(path, "/export")
		parts := strings.Split(strings.Trim(path, "/"), "/")

		if (len(parts) != 5 && !isExport) || (isExport && len(parts) != 6) {
			writeError(w, http.StatusBadRequest, "invalid_path", "路径格式错误")
			return
		}
		taskID := parts[4]

		if isExport {
			htmlDoc, err := history.ExportTaskHTML(r.Context(), taskID)
			if err != nil {
				writeError(w, http.StatusNotFound, "export_failed", err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(htmlDoc))
			return
		}

		task, steps, err := history.GetTaskDetail(r.Context(), taskID)
		if err != nil {
			writeError(w, http.StatusNotFound, "task_not_found", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"task":  task,
			"steps": steps,
		})
	})
}

func timeFromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error":   msg,
		"code":    code,
		"message": msg,
	})
}
