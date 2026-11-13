package companion

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"farm/internal/uixml"
)

type Status struct {
	Serial          string `json:"serial"`
	UpdateAvailable bool   `json:"update_available"`
	Installed       bool   `json:"installed"`
	Enabled         bool   `json:"enabled"`
	// Ready means enabled configuration and a successful protocol ping, not a readable current tree.
	Ready   bool   `json:"ready"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type unsupportedPackageVersionError struct{ code string }

func (e unsupportedPackageVersionError) Error() string {
	return fmt.Sprintf("unsupported installed companion versionCode %s (expected 2); update the Farm Companion APK", e.code)
}

var versionCodePattern = regexp.MustCompile(`(?m)\bversionCode=(\d+)\b`)
var versionNamePattern = regexp.MustCompile(`(?m)^\s*versionName=(\S+)`)

func (s *Service) Status(ctx context.Context, serial string) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	_, release, err := s.acquire(ctx, serial, false)
	if err != nil {
		return Status{Serial: serial, Error: err.Error()}, err
	}
	defer release()
	return s.statusLocked(ctx, serial, true)
}

func (s *Service) statusLocked(ctx context.Context, serial string, probe bool) (status Status, resultErr error) {
	status.Serial = serial
	defer func() {
		if resultErr != nil {
			status.Error = resultErr.Error()
		}
	}()
	out, err := s.command(ctx, serial, "shell", "pm", "list", "packages", Package)
	if err != nil {
		return status, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "package:"+Package {
			status.Installed = true
			break
		}
	}
	if !status.Installed {
		return status, nil
	}
	out, err = s.command(ctx, serial, "shell", "dumpsys", "package", Package)
	if err != nil {
		return status, err
	}
	code := versionCodePattern.FindStringSubmatch(string(out))
	if len(code) != 2 {
		status.UpdateAvailable = true
		return status, fmt.Errorf("installed companion versionCode is unavailable; update the Farm Companion APK")
	}
	status.Version = code[1]
	if name := versionNamePattern.FindStringSubmatch(string(out)); len(name) == 2 {
		status.Version = name[1]
	}
	var versionErr error
	if code[1] != "2" {
		status.UpdateAvailable = true
		versionErr = unsupportedPackageVersionError{code: code[1]}
	}
	services, err := s.enabledServices(ctx, serial)
	if err != nil {
		return status, errors.Join(versionErr, err)
	}
	master, err := s.command(ctx, serial, "shell", "settings", "get", "secure", "accessibility_enabled")
	if err != nil {
		return status, errors.Join(versionErr, err)
	}
	status.Enabled = containsService(services) && strings.TrimSpace(string(master)) == "1"
	if versionErr != nil {
		return status, versionErr
	}
	if status.Enabled && probe {
		if err := s.pingLocked(ctx, serial); err != nil {
			return status, err
		}
		status.Ready = true
	}
	return status, nil
}

// changeServices always reads immediately before writing and verifies the exact
// resulting list. It never removes another accessibility service.
func (s *Service) changeServices(ctx context.Context, serial string, enable bool) (before, after string, resultErr error) {
	before, err := s.enabledServices(ctx, serial)
	if err != nil {
		return "", "", err
	}
	entries := []string{}
	for _, entry := range strings.Split(before, ":") {
		if entry == "" {
			continue
		}
		if entry == Component || entry == Package+"/"+Package+".FarmAccessibilityService" {
			continue
		}
		entries = append(entries, entry)
	}
	if enable {
		entries = append(entries, Component)
	}
	after = strings.Join(entries, ":")
	if after != before {
		if _, err := s.command(ctx, serial, "shell", "settings", "put", "secure", "enabled_accessibility_services", shellQuote(after)); err != nil {
			return before, after, err
		}
	}
	actual, err := s.enabledServices(ctx, serial)
	if err != nil {
		return before, after, err
	}
	if actual != after {
		return before, after, fmt.Errorf("accessibility services changed concurrently; expected %q, got %q", after, actual)
	}
	if enable {
		if _, err := s.command(ctx, serial, "shell", "settings", "put", "secure", "accessibility_enabled", "1"); err != nil {
			return before, after, err
		}
		master, err := s.command(ctx, serial, "shell", "settings", "get", "secure", "accessibility_enabled")
		if err != nil {
			return before, after, err
		}
		if strings.TrimSpace(string(master)) != "1" {
			return before, after, fmt.Errorf("accessibility_enabled was not applied")
		}
	}
	return before, after, nil
}

func (s *Service) Enable(ctx context.Context, serial string) (Status, error) {
	return s.action(ctx, serial, "enable")
}
func (s *Service) Disable(ctx context.Context, serial string) (Status, error) {
	return s.action(ctx, serial, "disable")
}
func (s *Service) Install(ctx context.Context, serial string) (Status, error) {
	return s.action(ctx, serial, "install")
}

func (s *Service) action(ctx context.Context, serial, action string) (status Status, resultErr error) {
	actionTimeout := 30 * time.Second
	if action == "install" {
		actionTimeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	_, release, err := s.acquire(ctx, serial, true)
	if err != nil {
		return Status{Serial: serial, Error: err.Error()}, err
	}
	defer release()
	started := time.Now()
	defer func() {
		log.Printf("[Companion] action serial=%s action=%s elapsed_ms=%d error=%v", serial, action, time.Since(started).Milliseconds(), resultErr)
	}()
	switch action {
	case "install":
		if len(s.apk) == 0 {
			return status, fmt.Errorf("bundled companion APK is unavailable")
		}
		file, err := os.CreateTemp("", "farm-companion-*.apk")
		if err != nil {
			return status, err
		}
		path := file.Name()
		defer os.Remove(path)
		if _, err := file.Write(s.apk); err != nil {
			file.Close()
			return status, err
		}
		if err := file.Close(); err != nil {
			return status, err
		}
		out, err := s.command(ctx, serial, "install", "-r", path)
		if err != nil {
			return status, fmt.Errorf("install companion failed; existing app was retained (no uninstall): %w", err)
		}
		success := false
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) == "Success" {
				success = true
			}
		}
		if !success {
			return status, fmt.Errorf("adb install did not report Success: %s", strings.TrimSpace(string(out)))
		}
		status, err = s.statusLocked(ctx, serial, false)
		if err != nil {
			return status, err
		}
		if !status.Installed {
			return status, fmt.Errorf("install reported success but companion package is absent")
		}
		return status, nil
	case "enable":
		status, err = s.statusLocked(ctx, serial, false)
		if err != nil {
			return status, err
		}
		if !status.Installed {
			return status, fmt.Errorf("install the companion before enabling it")
		}
		if _, _, err := s.changeServices(ctx, serial, true); err != nil {
			return status, err
		}
		status.Enabled = true
		status.Ready = false
		readyCtx, readyCancel := context.WithTimeout(ctx, 10*time.Second)
		defer readyCancel()
		if err := s.waitReadyLocked(readyCtx, serial); err != nil {
			status.Error = err.Error()
			return status, fmt.Errorf("companion enabled but readiness verification failed: %w", err)
		}
		status.Ready = true
		return status, nil
	case "disable":
		if _, _, err := s.changeServices(ctx, serial, false); err != nil {
			return status, err
		}
		status, err = s.statusLocked(ctx, serial, false)
		if err != nil {
			// Removing our service works for old APKs too. Suppress only the
			// exact version diagnostic after settings were actually reread;
			// joined settings/transport failures must still fail this action.
			if _, onlyVersion := err.(unsupportedPackageVersionError); onlyVersion && !status.Enabled {
				return status, nil
			}
			return status, err
		}
		if status.Enabled {
			return status, fmt.Errorf("companion remained enabled")
		}
		return status, nil
	default:
		return status, fmt.Errorf("unsupported companion action %q", action)
	}
}

// FetchWithFallback coordinates temporary disabling and restoration with user
// actions. A queued explicit action invalidates restoration before it acquires
// this device gate, and external settings edits also prevent overwriting them.
func (s *Service) FetchWithFallback(ctx context.Context, serial string, fallback func(context.Context, string) (uixml.Snapshot, error)) (snapshot uixml.Snapshot, resultErr error) {
	g, release, err := s.acquire(ctx, serial, false)
	if err != nil {
		return snapshot, err
	}
	defer release()
	probeCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	snapshot, err = s.fetchLocked(probeCtx, serial)
	cancel()
	if err == nil {
		return snapshot, nil
	}
	log.Printf("[Companion] falling back serial=%s error=%v", serial, err)
	if ctx.Err() != nil {
		return snapshot, ctx.Err()
	}
	services, settingsErr := s.enabledServices(ctx, serial)
	if settingsErr != nil {
		return snapshot, fmt.Errorf("companion failed (%v); cannot safely prepare dump: %w", err, settingsErr)
	}
	originalMaster, masterErr := s.accessibilityMaster(ctx, serial)
	if masterErr != nil {
		return snapshot, fmt.Errorf("cannot safely read accessibility state before dump: %w", masterErr)
	}
	if containsService(services) && originalMaster == "1" {
		revision := s.revision(g)
		before, temporary, err := s.changeServices(ctx, serial, false)
		if err != nil {
			return snapshot, fmt.Errorf("temporarily disable companion before dump: %w", err)
		}
		defer func() {
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), restoreTimeout)
			defer restoreCancel()
			restoreFailed := func(err error) {
				log.Printf("[Companion] RESTORE FAILED serial=%s error=%v", serial, err)
				resultErr = errors.Join(resultErr, fmt.Errorf("companion restoration failed after dump: %w", err))
			}
			if err := s.restoreLocked(restoreCtx, serial, g, revision, before, temporary, originalMaster); err != nil {
				restoreFailed(err)
			}
		}()
	}
	snapshot, err = fallback(ctx, serial)
	if err == nil {
		snapshot.Source = "uiautomator"
	}
	return snapshot, err
}

// restoreLocked verifies lifecycle configuration and liveness independently of
// the window tree that caused fallback. The caller owns the device gate.
func (s *Service) restoreLocked(ctx context.Context, serial string, g *deviceGate, revision uint64, before, temporary, originalMaster string) error {
	if s.revision(g) != revision {
		log.Printf("[Companion] restore skipped for queued user action serial=%s", serial)
		return nil
	}
	actual, err := s.enabledServices(ctx, serial)
	if err != nil {
		return err
	}
	if actual != temporary {
		log.Printf("[Companion] restore skipped after external settings change serial=%s", serial)
		return nil
	}
	if _, err := s.command(ctx, serial, "shell", "settings", "put", "secure", "enabled_accessibility_services", shellQuote(before)); err != nil {
		return err
	}
	if _, err := s.command(ctx, serial, "shell", "settings", "put", "secure", "accessibility_enabled", originalMaster); err != nil {
		return err
	}
	actual, err = s.enabledServices(ctx, serial)
	if err != nil {
		return err
	}
	master, err := s.accessibilityMaster(ctx, serial)
	if err != nil {
		return err
	}
	if actual != before || master != originalMaster {
		return fmt.Errorf("restored settings mismatch services=%q accessibility_enabled=%q", actual, master)
	}
	if err := s.waitReadyLocked(ctx, serial); err != nil {
		return fmt.Errorf("restored settings but companion ping failed: %w", err)
	}
	return nil
}
