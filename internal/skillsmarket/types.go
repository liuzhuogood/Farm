package skillsmarket

import "strings"

// ContractPath is the agent-facing automation API reference. Installed skills
// read it to learn the exact endpoints instead of guessing the full OpenAPI.
const ContractPath = "/fram.skill"

// ContractURL builds the absolute contract URL for the given listen address.
// It mirrors mcpconfig.Endpoint so both advertised URLs use the same host.
func ContractURL(addr string, https bool) string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	addr = strings.Replace(addr, "0.0.0.0:", "127.0.0.1:", 1)
	addr = strings.Replace(addr, "[::]:", "127.0.0.1:", 1)
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return scheme + "://" + addr + ContractPath
}

// AiToolID represents a supported AI assistant tool.
type AiToolID string

const (
	ToolCodex      AiToolID = "codex"
	ToolClaudeCode AiToolID = "claude-code"
	ToolWorkbuddy  AiToolID = "workbuddy"
)

// AiTool represents metadata, guide steps, and installation paths for an AI assistant.
type AiTool struct {
	ID              AiToolID `json:"id"`
	Name            string   `json:"name"`
	Subtitle        string   `json:"subtitle"`
	GuideTitle      string   `json:"guideTitle"`
	GuideBadge      string   `json:"guideBadge"`
	Step1Title      string   `json:"step1Title"`
	Step1Desc       string   `json:"step1Desc"`
	Step2Title      string   `json:"step2Title"`
	Step2Desc       string   `json:"step2Desc"`
	Step2ActionText string   `json:"step2ActionText"`
	Step3Title      string   `json:"step3Title"`
	Step3Desc       string   `json:"step3Desc"`
	Step3ActionText string   `json:"step3ActionText"`
	SamplePrompt    string   `json:"samplePrompt"`
	IconType        string   `json:"iconType"`
	TargetPath      string   `json:"targetPath"`
	TargetRelPath   string   `json:"targetRelPath"`
	OfficialDocURL  string   `json:"officialDocUrl"`
}

// AiIntegrationsResponse is the response structure for GET /api/ai-integrations.
type AiIntegrationsResponse struct {
	Tools []AiTool `json:"tools"`
	// ContractURL points to the agent-facing automation API reference that
	// installed skills must read before calling any endpoint.
	ContractURL string `json:"contractUrl"`
}

// SkillID represents an available skill identifier.
type SkillID string

const (
	SkillDeviceBase    SkillID = "farm-device-base"
	SkillKuaishouDaily SkillID = "farm-kuaishou-daily"
	SkillWechatHelper  SkillID = "farm-wechat-helper"
	SkillCommon        SkillID = "my-scrcpy-common"
)

// SkillCapability represents a specific capability of a skill.
type SkillCapability struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// SkillDetails contains full details for a skill.
type SkillDetails struct {
	Overview      string            `json:"overview"`
	Capabilities  []SkillCapability `json:"capabilities"`
	Scenarios     []string          `json:"scenarios"`
	SamplePrompts []string          `json:"samplePrompts"`
	Prerequisites []string          `json:"prerequisites"`
	UsageSteps    []string          `json:"usageSteps"`
}

// TargetStatus describes the installation status of a skill for a specific AI assistant.
type TargetStatus struct {
	Installed bool   `json:"installed"`
	Status    string `json:"status"` // "installed", "not_installed", "conflict", "error"
	Path      string `json:"path"`
	Error     string `json:"error,omitempty"`
}

// SkillSummary represents summary info and target statuses for a skill.
type SkillSummary struct {
	ID               SkillID                   `json:"id"`
	Name             string                    `json:"name"`
	App              string                    `json:"app"`
	Category         string                    `json:"category"`
	CategoryLabel    string                    `json:"categoryLabel"`
	Description      string                    `json:"description"`
	IconType         string                    `json:"iconType"`
	ZipDownloadURL   string                    `json:"zipDownloadUrl"`
	ZipFileName      string                    `json:"zipFileName"`
	InstalledTargets []AiToolID                `json:"installedTargets"`
	TargetStatuses   map[AiToolID]TargetStatus `json:"targetStatuses"`
}

// SkillDetail represents full info for a skill.
type SkillDetail struct {
	SkillSummary
	Details SkillDetails `json:"details"`
}

// SkillsListResponse is the response structure for GET /api/skills.
type SkillsListResponse struct {
	Skills []SkillSummary `json:"skills"`
}

// SkillInstallRequest is the request structure for POST /api/skills/{id}/install.
type SkillInstallRequest struct {
	Targets []AiToolID `json:"targets"`
}

// SkillInstallTargetResult describes the installation result for a single target.
type SkillInstallTargetResult struct {
	Target AiToolID `json:"target"`
	Status string   `json:"status"` // "installed", "already_installed", "failed"
	Path   string   `json:"path"`
	Error  string   `json:"error,omitempty"`
}

// SkillInstallResponse is the response structure for POST /api/skills/{id}/install.
type SkillInstallResponse struct {
	SkillID SkillID                    `json:"skillId"`
	Results []SkillInstallTargetResult `json:"results"`
}
