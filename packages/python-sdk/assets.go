package pythonsdk

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Include only installation inputs, excluding developer environments, build
// output and package metadata left by local Python tooling.
//
//go:embed pyproject.toml README.md src/farm/*.py
var source embed.FS

// ExtractSource prepares the exact SDK shipped with the service for standard
// pip installation. The caller must clean it up after pip has completed.
func ExtractSource() (string, func(), error) {
	directory, err := os.MkdirTemp("", "farm-sdk-")
	if err != nil {
		return "", nil, fmt.Errorf("创建 SDK 安装目录失败: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	err = fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		destination := filepath.Join(directory, filepath.FromSlash(name))
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		content, err := source.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0600)
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("解包内置 SDK 失败: %w", err)
	}
	return directory, cleanup, nil
}
