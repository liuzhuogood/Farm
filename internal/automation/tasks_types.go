package automation

import "time"

// Task is the device panel's snapshot. Active means an act batch is executing;
// running with Active=false is an explicit task waiting for its next batch.
type Task struct {
	ID             string         `json:"id"`
	Serial         string         `json:"serial"`
	Title          string         `json:"title"`
	Source         string         `json:"source"`
	State          string         `json:"state"`
	Summary        string         `json:"summary"`
	StartedAt      time.Time      `json:"started_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	EndedAt        *time.Time     `json:"ended_at,omitempty"`
	Active         bool           `json:"active"`
	Steps          []TaskStep     `json:"steps"`
	StepsTruncated bool           `json:"steps_truncated,omitempty"`
	Artifacts      []TaskArtifact `json:"artifacts"`
	Directory      string         `json:"directory,omitempty"`
}

type TaskStep struct {
	reserve   int64
	Index     int            `json:"index"`
	Line      int            `json:"line,omitempty"`
	Title     string         `json:"title"`
	Command   string         `json:"command"`
	Status    string         `json:"status"`
	StartedAt *time.Time     `json:"started_at,omitempty"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	ElapsedMS int64          `json:"elapsed_ms"`
	Result    string         `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Artifacts []TaskArtifact `json:"artifacts"`
}

type TaskArtifact struct {
	ID           string `json:"id"`
	Serial       string `json:"serial"`
	RelativePath string `json:"relative_path"`
	Kind         string `json:"kind"`
	Size         int64  `json:"size,omitempty"`
	DownloadURL  string `json:"download_url"`
	PreviewURL   string `json:"preview_url,omitempty"`
	Removed      bool   `json:"removed,omitempty"`
}
