package updater

import (
	"fmt"
	"log"
	"strings"
)

// 更新通道：正式通道只取正式发布，beta 通道额外包含预发布。
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"

	channelConfigKey = "update_channel"

	// desktopStableEndpoint 指向最新正式发布的桌面更新索引。
	desktopStableEndpoint = "https://github.com/liuzhuogood/Farm/releases/latest/download/desktop-latest.json"
	desktopManifestName   = "desktop-latest.json"

	// releaseListSize 是 beta 通道向前回溯的发布条数。
	releaseListSize = 30
)

// ConfigStore 持久化更新通道，与系统配置共用同一张表。
type ConfigStore interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
}

// NormalizeChannel 校验通道取值；空值表示默认的正式通道。
func NormalizeChannel(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ChannelStable:
		return ChannelStable, nil
	case ChannelBeta:
		return ChannelBeta, nil
	default:
		return "", fmt.Errorf("未知更新通道: %s", strings.TrimSpace(value))
	}
}

// isBetaTag 判断 tag 是否为 beta 预发布；只认 `-beta.N`，避免把历史补丁号当成 beta。
func isBetaTag(tag string) bool {
	return strings.Contains(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "-beta.")
}

// DesktopEndpoint 返回桌面更新索引地址与实际生效的通道。
// 正式通道固定指向最新正式发布；beta 通道只在存在 beta 预发布时切换，否则回退正式通道。
func (c *Checker) DesktopEndpoint(channel string) (endpoint string, effective string, err error) {
	normalized, err := NormalizeChannel(channel)
	if err != nil {
		return "", "", err
	}
	if normalized == ChannelStable {
		return desktopStableEndpoint, ChannelStable, nil
	}
	releases, err := c.listReleases()
	if err != nil {
		return "", "", fmt.Errorf("获取 beta 发布列表失败: %w", err)
	}
	var best *ReleaseInfo
	for i := range releases {
		release := &releases[i]
		if release.Draft || !release.Prerelease || !isBetaTag(release.TagName) {
			continue
		}
		if best == nil || CompareVersions(release.TagName, best.TagName) > 0 {
			best = release
		}
	}
	if best == nil {
		log.Printf("[Updater] 当前没有 beta 发布，桌面更新回退正式通道")
		return desktopStableEndpoint, ChannelStable, nil
	}
	endpoint = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", c.repo, best.TagName, desktopManifestName)
	return endpoint, ChannelBeta, nil
}
