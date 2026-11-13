package automation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const taskPersistenceLimit = 32 << 20

type taskPersistenceDocument struct {
	Version int    `json:"version"`
	Tasks   []Task `json:"tasks"`
}

// One worker and one coalescing signal bound pending persistence work. Runtime
// mutations never wait for disk; every write reads the latest complete snapshot.
type automationTaskPersistence struct {
	file      string
	snapshot  func() []Task
	signal    chan struct{}
	stop      chan struct{}
	done      chan struct{}
	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
}

func openTaskPersistence(file string, snapshot func() []Task) (*automationTaskPersistence, []Task, error) {
	if file == "" {
		return nil, []Task{}, nil
	}
	if snapshot == nil {
		return nil, nil, fmt.Errorf("task snapshot callback is required")
	}
	tasks, err := loadTaskPersistence(file)
	if err != nil {
		return nil, nil, err
	}
	p := &automationTaskPersistence{file: file, snapshot: snapshot, signal: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go p.run()
	return p, tasks, nil
}

func loadTaskPersistence(file string) ([]Task, error) {
	f, err := os.Open(file)
	if os.IsNotExist(err) {
		return []Task{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read task history: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, taskPersistenceLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read task history: %w", err)
	}
	if len(data) > taskPersistenceLimit {
		return nil, fmt.Errorf("task history exceeds %d bytes", taskPersistenceLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document taskPersistenceDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode task history: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("task history must contain exactly one JSON document")
	}
	if document.Version != 1 {
		return nil, fmt.Errorf("unsupported task history version %d", document.Version)
	}
	if len(document.Tasks) > 272 {
		return nil, fmt.Errorf("task history exceeds task capacity")
	}
	seen := make(map[string]bool, len(document.Tasks))
	open, completed := 0, 0
	now := time.Now().UTC()
	for i := range document.Tasks {
		task := &document.Tasks[i]
		if err := validatePersistedTask(*task); err != nil {
			return nil, fmt.Errorf("invalid persisted task %q: %w", task.ID, err)
		}
		if seen[task.ID] {
			return nil, fmt.Errorf("duplicate persisted task %q", task.ID)
		}
		seen[task.ID] = true
		if task.State == "running" {
			open++
		} else {
			completed++
		}
		if open > 16 || completed > 256 {
			return nil, fmt.Errorf("task history exceeds open/completed capacity")
		}
		task.Active = false
		if task.State == "running" {
			task.State = "stopped"
			task.UpdatedAt = now
			ended := now
			task.EndedAt = &ended
			if task.Summary != "" {
				task.Summary += "\n"
			}
			task.Summary += "服务重启，任务已停止；重启前未完成步骤的执行结果未确认。"
			for j := range task.Steps {
				step := &task.Steps[j]
				switch step.Status {
				case "running":
					step.Status = "failed"
					step.Error = "服务重启，步骤执行结果未确认。"
					ended := now
					step.EndedAt = &ended
				case "pending":
					step.Status = "skipped"
					step.Error = "服务重启，待执行步骤已跳过。"
					ended := now
					step.EndedAt = &ended
				}
			}
		}
	}
	if document.Tasks == nil {
		document.Tasks = []Task{}
	}
	return document.Tasks, nil
}

func validatePersistedTask(task Task) error {
	if task.ID == "" || task.Serial == "" || strings.TrimSpace(task.Title) == "" || task.Source != "automation" {
		return fmt.Errorf("id, serial, title and automation source are required")
	}
	if task.StartedAt.IsZero() || task.UpdatedAt.IsZero() {
		return fmt.Errorf("task timestamps are required")
	}
	switch task.State {
	case "running", "done", "failed", "stopped":
	default:
		return fmt.Errorf("unknown task state %q", task.State)
	}
	if len(task.Steps) > 500 {
		return fmt.Errorf("step capacity exceeded")
	}
	if task.Directory != "" {
		if _, err := deviceArtifactDirectory(task.Directory); err != nil {
			return err
		}
	}
	previous := 0
	for _, step := range task.Steps {
		if step.Index <= previous {
			return fmt.Errorf("step indices must be positive and increasing")
		}
		previous = step.Index
		switch step.Status {
		case "pending", "running", "done", "failed", "skipped":
		default:
			return fmt.Errorf("unknown step state %q", step.Status)
		}
		for _, artifact := range step.Artifacts {
			if err := validatePersistedTaskArtifact(task.Serial, artifact); err != nil {
				return err
			}
		}
	}
	for _, artifact := range task.Artifacts {
		if err := validatePersistedTaskArtifact(task.Serial, artifact); err != nil {
			return err
		}
	}
	return nil
}

func validatePersistedTaskArtifact(serial string, artifact TaskArtifact) error {
	if artifact.ID == "" || artifact.Serial != serial {
		return fmt.Errorf("artifact must belong to the task device")
	}
	if _, err := deviceArtifactRelative(artifact.RelativePath); err != nil {
		return err
	}
	switch artifact.Kind {
	case "image", "video", "file":
	default:
		return fmt.Errorf("unknown artifact kind %q", artifact.Kind)
	}
	if artifact.Size < 0 {
		return fmt.Errorf("artifact size must not be negative")
	}
	return nil
}

func (p *automationTaskPersistence) Schedule() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	select {
	case p.signal <- struct{}{}:
	default:
	}
}

func (p *automationTaskPersistence) run() {
	defer close(p.done)
	write := func() {
		if err := writeTaskPersistence(p.file, p.snapshot()); err != nil {
			log.Printf("[AutomationTasks] 内存任务状态已更新，但磁盘持久化失败 file=%q: %v", p.file, err)
		}
	}
	for {
		select {
		case <-p.stop:
			write()
			return
		case <-p.signal:
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-p.stop:
				timer.Stop()
				write()
				return
			case <-timer.C:
				write()
			}
		}
	}
}

func writeTaskPersistence(file string, tasks []Task) error {
	data, err := json.Marshal(taskPersistenceDocument{Version: 1, Tasks: tasks})
	if err != nil {
		return fmt.Errorf("encode task history: %w", err)
	}
	if len(data) > taskPersistenceLimit {
		return fmt.Errorf("task snapshot exceeds %d bytes", taskPersistenceLimit)
	}
	directory := filepath.Dir(file)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tasks-*.partial")
	if err != nil {
		return err
	}
	defer func() { temporary.Close(); os.Remove(temporary.Name()) }()
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), file); err != nil {
		return err
	}
	return nil
}

func (p *automationTaskPersistence) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() { p.mu.Lock(); p.closed = true; close(p.stop); p.mu.Unlock() })
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		log.Printf("[AutomationTasks] 退出等待磁盘持久化超过5秒；内存状态更新与磁盘写入独立，磁盘快照可能尚未更新 file=%q", p.file)
	}
}
