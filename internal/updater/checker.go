package updater

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGitHubRepo = "liuzhuogood/Farm"
	githubAPIBase     = "https://api.github.com/repos"
)

// ReleaseAsset 代表 GitHub Release 中的一个资源
type ReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

// ReleaseInfo 代表 GitHub Release 的精简信息
type ReleaseInfo struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt time.Time      `json:"published_at"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []ReleaseAsset `json:"assets"`
}

// CheckResult 是检查更新返回给前端的数据结构
type CheckResult struct {
	Policy
	CanUpgrade     bool   `json:"can_upgrade"`
	Digest         string `json:"digest"`
	Channel        string `json:"channel"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
	AssetName      string `json:"asset_name"`
	AssetSize      int64  `json:"asset_size"`
	DownloadURL    string `json:"download_url"`
}

// Checker 负责检查 GitHub Release 更新
type Checker struct {
	repo           string
	currentVersion string
	channel        string
	httpClient     *http.Client
}

// NewChecker 创建 Checker 实例
func NewChecker(currentVersion string) *Checker {
	v := strings.TrimSpace(currentVersion)
	if v == "" {
		// 尝试从 VERSION 文件读取
		if data, err := os.ReadFile("VERSION"); err == nil {
			v = strings.TrimSpace(string(data))
		}
	}
	if v == "" {
		v = "v0.0.0"
	}
	return &Checker{
		repo:           defaultGitHubRepo,
		currentVersion: v,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SetRepo 自定义仓库，方便测试
func (c *Checker) SetRepo(repo string) {
	c.repo = repo
}

// GetCurrentVersion 获取当前版本号
func (c *Checker) GetCurrentVersion() string {
	return c.currentVersion
}

// SetChannel 切换检查更新使用的通道。
func (c *Checker) SetChannel(channel string) {
	c.channel = channel
}

// Channel 返回当前通道，空值按正式通道处理。
func (c *Checker) Channel() string {
	if channel, err := NormalizeChannel(c.channel); err == nil {
		return channel
	}
	return ChannelStable
}

// CheckUpdate 按当前通道查询最新发布并对比版本
func (c *Checker) CheckUpdate() (*CheckResult, error) {
	return c.CheckUpdateChannel(c.Channel())
}

// CheckUpdateChannel 按指定通道查询最新发布并对比版本。
func (c *Checker) CheckUpdateChannel(channel string) (*CheckResult, error) {
	release, err := c.latestRelease(channel)
	if err != nil {
		return nil, err
	}

	hasUpdate := CompareVersions(release.TagName, c.currentVersion) > 0

	// 匹配当前平台适合的 asset
	targetAsset := MatchAsset(release.Assets, runtime.GOOS, runtime.GOARCH)

	result := &CheckResult{
		Channel:        channel,
		CurrentVersion: c.currentVersion,
		LatestVersion:  release.TagName,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt.Format(time.RFC3339),
	}

	if targetAsset != nil {
		result.AssetName = targetAsset.Name
		result.AssetSize = targetAsset.Size
		result.DownloadURL = targetAsset.BrowserDownloadURL
		result.Digest = targetAsset.Digest
	}

	return result, nil
}

// latestRelease 取当前通道可见的最新发布：正式通道只看正式发布，beta 通道取 beta 与正式中较新的一个。
func (c *Checker) latestRelease(channel string) (*ReleaseInfo, error) {
	if channel == ChannelBeta {
		releases, err := c.listReleases()
		if err != nil {
			log.Printf("[Updater] 获取 beta 发布列表失败，回退正式通道: %v", err)
		} else if best := pickRelease(releases, true); best != nil {
			log.Printf("[Updater] beta 通道选中发布: %s", best.TagName)
			return best, nil
		}
	}
	return c.latestStable()
}

func (c *Checker) latestStable() (*ReleaseInfo, error) {
	var release ReleaseInfo
	url := fmt.Sprintf("%s/%s/releases/latest", githubAPIBase, c.repo)
	if err := c.getJSON(url, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (c *Checker) listReleases() ([]ReleaseInfo, error) {
	var releases []ReleaseInfo
	url := fmt.Sprintf("%s/%s/releases?per_page=%d", githubAPIBase, c.repo, releaseListSize)
	if err := c.getJSON(url, &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func (c *Checker) getJSON(url string, target any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Farm-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github api returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode release info failed: %w", err)
	}
	return nil
}

// pickRelease 在发布列表中选出可见的最新版本；草稿与不匹配的预发布不计入。
func pickRelease(releases []ReleaseInfo, includeBeta bool) *ReleaseInfo {
	var best *ReleaseInfo
	for i := range releases {
		release := &releases[i]
		if release.Draft {
			continue
		}
		if release.Prerelease && (!includeBeta || !isBetaTag(release.TagName)) {
			continue
		}
		if best == nil || CompareVersions(release.TagName, best.TagName) > 0 {
			best = release
		}
	}
	return best
}

// MatchAsset 根据操作系统和架构匹配 GitHub Release 中的对应文件名
func MatchAsset(assets []ReleaseAsset, goos, goarch string) *ReleaseAsset {
	var expectedSuffix string
	switch {
	case goos == "linux" && goarch == "amd64":
		expectedSuffix = "linux-amd64"
	case goos == "linux" && goarch == "arm64":
		expectedSuffix = "linux-arm64"
	case goos == "darwin" && goarch == "amd64":
		expectedSuffix = "macos-amd64"
	case goos == "darwin" && goarch == "arm64":
		expectedSuffix = "macos-arm64"
	case goos == "windows" && goarch == "amd64":
		expectedSuffix = "windows-amd64.exe"
	default:
		expectedSuffix = fmt.Sprintf("%s-%s", goos, goarch)
	}

	cliSuffix := expectedSuffix + "-cli"
	if goos == "windows" && goarch == "amd64" {
		cliSuffix = "windows-cli-amd64.exe"
	}
	// Prefer the CLI distribution. The exact historical name remains supported
	// for releases published before CLI and GUI assets were separated.
	for _, name := range []string{"farm-" + cliSuffix, "farm-" + expectedSuffix} {
		for _, a := range assets {
			if a.Name == name {
				return &a
			}
		}
	}
	return nil
}

// CompareVersions 比较两个版本号，返回 1 表示 v1 更新、-1 表示 v2 更新、0 表示相等。
//
// 本项目沿用「日期 + 同日序号」编号，因此后缀优先级与标准 semver 不同：
// `-beta.N` 低于同号正式版，同日补丁 `-N` 高于同号正式版。
// 例：v2026.11.20-beta.2 < v2026.11.20 < v2026.11.20-1 < v2026.11.20-2 < v2026.11.21。
func CompareVersions(v1, v2 string) int {
	key1, key2 := parseVersionKey(v1), parseVersionKey(v2)
	length := len(key1.main)
	if len(key2.main) > length {
		length = len(key2.main)
	}
	for i := 0; i < length; i++ {
		if diff := versionPart(key1.main, i) - versionPart(key2.main, i); diff != 0 {
			return versionSign(diff)
		}
	}
	if key1.level != key2.level {
		return versionSign(key1.level - key2.level)
	}
	return versionSign(key1.order - key2.order)
}

const (
	// 后缀分级：beta 预发布 < 正式首版 < 同日补丁。
	versionLevelBeta    = 1
	versionLevelRelease = 2
	versionLevelPatch   = 3
)

type versionKey struct {
	main  []int
	level int
	order int
}

func parseVersionKey(v string) versionKey {
	clean := strings.TrimPrefix(strings.TrimSpace(v), "v")
	if index := strings.Index(clean, "+"); index >= 0 {
		clean = clean[:index]
	}
	mainPart, suffix, _ := strings.Cut(clean, "-")
	key := versionKey{main: parseVersionParts(mainPart), level: versionLevelRelease}
	if suffix == "" {
		return key
	}
	lower := strings.ToLower(suffix)
	if trimmed, ok := strings.CutPrefix(lower, "beta"); ok {
		key.level = versionLevelBeta
		key.order = trailingNumber(strings.TrimPrefix(trimmed, "."))
		return key
	}
	if number, err := strconv.Atoi(lower); err == nil {
		key.level = versionLevelPatch
		key.order = number
		return key
	}
	// 其它预发布写法按 beta 级别处理，只按后缀里的数字排序。
	key.level = versionLevelBeta
	key.order = trailingNumber(lower)
	return key
}

func trailingNumber(value string) int {
	end := len(value)
	start := end
	for start > 0 && value[start-1] >= '0' && value[start-1] <= '9' {
		start--
	}
	if start == end {
		return 0
	}
	number, err := strconv.Atoi(value[start:end])
	if err != nil {
		return 0
	}
	return number
}

func versionPart(values []int, index int) int {
	if index < len(values) {
		return values[index]
	}
	return 0
}

func versionSign(value int) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}

func parseVersionParts(v string) []int {
	// 去除 prerelease / metadata，例如 1.13.0-rc1 -> 1.13.0
	mainPart := strings.Split(v, "-")[0]
	mainPart = strings.Split(mainPart, "+")[0]
	strParts := strings.Split(mainPart, ".")
	var parts []int
	for _, p := range strParts {
		n, err := strconv.Atoi(p)
		if err != nil {
			parts = append(parts, 0)
		} else {
			parts = append(parts, n)
		}
	}
	return parts
}
