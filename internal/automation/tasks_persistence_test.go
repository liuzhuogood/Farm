package automation

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func persistenceTestTask(id, state string) Task {
	now := time.Now().UTC()
	task := Task{ID: id, Serial: "phone", Title: "任务持久化", Source: "automation", State: state, StartedAt: now, UpdatedAt: now, Steps: []TaskStep{}, Artifacts: []TaskArtifact{}, Directory: "tasks/" + id}
	if state != "running" {
		ended := now
		task.EndedAt = &ended
	}
	return task
}

func TestTaskPersistenceRestartsRunningTasksAsStopped(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tasks.json")
	running := persistenceTestTask("task-running", "running")
	running.Active = true
	started := running.StartedAt
	running.Steps = []TaskStep{
		{Index: 1, Title: "创建目录", Command: "mkdir tasks/task-running", Status: "done", Artifacts: []TaskArtifact{}},
		{Index: 2, Title: "录屏", Command: "record 5 tasks/task-running", Status: "running", StartedAt: &started, Artifacts: []TaskArtifact{}},
		{Index: 3, Title: "下一步", Command: "keyevent back", Status: "pending", Artifacts: []TaskArtifact{}},
	}
	artifact := TaskArtifact{ID: "artifact-1", Serial: "phone", RelativePath: "tasks/task-running/shot.png", Kind: "image", Size: 123, DownloadURL: "/api/automation/artifacts/download?serial=phone&directory=tasks%2Ftask-running&file=shot.png", PreviewURL: "/api/automation/artifacts/download?preview=1"}
	running.Artifacts = []TaskArtifact{artifact}
	running.Steps[0].Artifacts = []TaskArtifact{artifact}
	done := persistenceTestTask("task-done", "done")
	done.Summary = "已经完成"
	if err := writeTaskPersistence(file, []Task{running, done}); err != nil {
		t.Fatal(err)
	}
	var snapshot []Task
	p, loaded, err := openTaskPersistence(file, func() []Task { return snapshot })
	if err != nil {
		t.Fatal(err)
	}
	snapshot = loaded
	if len(loaded) != 2 {
		t.Fatalf("loaded=%+v", loaded)
	}
	got := loaded[0]
	if got.State != "stopped" || got.Active || got.EndedAt == nil || !strings.Contains(got.Summary, "服务重启") {
		t.Fatalf("unfinished task not recovered: %+v", got)
	}
	if got.Steps[0].Status != "done" || got.Steps[1].Status != "failed" || got.Steps[2].Status != "skipped" || got.Steps[1].Error == "" || got.Steps[2].Error == "" {
		t.Fatalf("unfinished steps not recovered: %+v", got.Steps)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0] != artifact {
		t.Fatalf("artifact metadata changed: %+v", got.Artifacts)
	}
	if loaded[1].State != "done" || loaded[1].Summary != "已经完成" {
		t.Fatalf("finished task changed: %+v", loaded[1])
	}
	p.Close()
	reloaded, err := loadTaskPersistence(file)
	if err != nil || reloaded[0].State != "stopped" {
		t.Fatalf("recovery was not persisted on close: %+v %v", reloaded, err)
	}
}

func TestTaskPersistenceCoalescesAndFlushesLatestSnapshot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested", "tasks.json")
	var revision atomic.Int64
	p, loaded, err := openTaskPersistence(file, func() []Task {
		task := persistenceTestTask("task-a", "done")
		task.Summary = strconv.FormatInt(revision.Load(), 10)
		return []Task{task}
	})
	if err != nil || len(loaded) != 0 {
		t.Fatalf("open=%v loaded=%+v", err, loaded)
	}
	finished := make(chan struct{})
	go func() {
		for i := int64(0); i < 10000; i++ {
			revision.Store(i)
			p.Schedule()
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduling persistence blocked runtime updates")
	}
	p.Close()
	p.Close()
	p.Schedule()
	tasks, err := loadTaskPersistence(file)
	if err != nil || len(tasks) != 1 || tasks[0].Summary != "9999" {
		t.Fatalf("latest state not flushed: %+v %v", tasks, err)
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tasks.json" {
		t.Fatalf("temporary persistence files leaked: %+v", entries)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("task metadata has broad permissions: %v", info.Mode())
	}
}

func TestTaskPersistenceRejectsCorruptOrOutOfBudgetHistory(t *testing.T) {
	valid := persistenceTestTask("task-a", "done")
	encode := func(tasks []Task) []byte {
		data, err := json.Marshal(taskPersistenceDocument{Version: 1, Tasks: tasks})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	badSource := valid
	badSource.Source = "mcp"
	badStep := valid
	badStep.Steps = []TaskStep{{Index: 1, Status: "invented"}}
	badArtifact := valid
	badArtifact.Artifacts = []TaskArtifact{{ID: "a", Serial: "other-phone", RelativePath: "shot.png", Kind: "image"}}
	badDirectory := valid
	badDirectory.Directory = "../escape"
	for name, data := range map[string][]byte{
		"unknown version":       []byte(`{"version":2,"tasks":[]}`),
		"unknown field":         []byte(`{"version":1,"tasks":[],"guess":true}`),
		"trailing document":     []byte(`{"version":1,"tasks":[]} {}`),
		"duplicate id":          encode([]Task{valid, valid}),
		"wrong source":          encode([]Task{badSource}),
		"wrong state":           encode([]Task{badStep}),
		"wrong artifact device": encode([]Task{badArtifact}),
		"directory escape":      encode([]Task{badDirectory}),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "tasks.json")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			if p, _, err := openTaskPersistence(file, func() []Task { return nil }); err == nil {
				p.Close()
				t.Fatal("invalid history accepted")
			}
			kept, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(data, kept) {
				t.Fatal("invalid history was overwritten")
			}
		})
	}
	t.Run("oversized file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "tasks.json")
		f, err := os.Create(file)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(taskPersistenceLimit + 1); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if p, _, err := openTaskPersistence(file, func() []Task { return nil }); err == nil {
			p.Close()
			t.Fatal("oversized history accepted")
		}
	})
}

func TestTaskPersistenceWriteFailureIsExplicitAndDoesNotBlock(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tasks.json")
	p, _, err := openTaskPersistence(file, func() []Task { return []Task{persistenceTestTask("task-a", "done")} })
	if err != nil {
		t.Fatal(err)
	}
	// Make atomic publication fail after encoding/writing the temporary file.
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	p.Schedule()
	p.Close()
	if !strings.Contains(output.String(), "内存任务状态已更新") || !strings.Contains(output.String(), "磁盘持久化失败") {
		t.Fatalf("persistence failure not explicit: %s", output.String())
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tasks.json" || !entries[0].IsDir() {
		t.Fatalf("failed atomic write left partial data: %+v", entries)
	}
}

func TestTaskPersistenceCanRemainInMemory(t *testing.T) {
	p, tasks, err := openTaskPersistence("", nil)
	if err != nil || p != nil || tasks == nil || len(tasks) != 0 {
		t.Fatalf("in-memory result=%+v %+v %v", p, tasks, err)
	}
	p.Schedule()
	p.Close()
}
