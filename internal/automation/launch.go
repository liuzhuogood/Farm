package automation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"farm/internal/dependencies"
)

var androidPackagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
var androidActivityPattern = regexp.MustCompile(`^[A-Za-z0-9_.$]+$`)

type launchRunner func(context.Context, ...string) ([]byte, error)

// LaunchApp resolves an explicit package's launcher, starts it and verifies the
// actual foreground package. Command success alone does not establish launch.
func (s *Service) LaunchApp(ctx context.Context, serial, pkg string) error {
	return s.launchAppWithRunner(ctx, serial, pkg, func(ctx context.Context, args ...string) ([]byte, error) {
		return dependencies.CommandContext(ctx, s.adbPath, args...).CombinedOutput()
	})
}

func (s *Service) launchAppWithRunner(ctx context.Context, serial, pkg string, run launchRunner) (resultErr error) {
	if strings.TrimSpace(serial) == "" {
		return fmt.Errorf("missing serial")
	}
	if !androidPackagePattern.MatchString(pkg) {
		return fmt.Errorf("app must be an explicit Android package name, for example com.android.settings")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	started := time.Now()
	defer func() {
		fields := AutomationLogFields("package", pkg, "elapsed_ms", time.Since(started).Milliseconds())
		kind, message := LogTypeAction, "应用启动且前台验证完成"
		if resultErr != nil {
			kind, message = LogTypeError, "应用启动验证失败"
			fields["error"] = resultErr.Error()
		}
		s.recordAutomationLog(AutomationLog{Type: kind, Serial: serial, Message: message, Fields: fields})
	}()

	out, err := run(ctx, "-s", serial, "shell", "cmd", "package", "resolve-activity", "--brief", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.LAUNCHER", "-p", pkg)
	if err != nil {
		return fmt.Errorf("resolve launcher: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var component string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		activity, ok := strings.CutPrefix(line, pkg+"/")
		if ok && androidActivityPattern.MatchString(activity) {
			if component != "" {
				return fmt.Errorf("ambiguous launcher for %s", pkg)
			}
			component = line
		}
	}
	if component == "" {
		return fmt.Errorf("no launcher activity for %s: %s", pkg, strings.TrimSpace(string(out)))
	}
	out, err = run(ctx, "-s", serial, "shell", "am", "start", "-W", "-n", component)
	if err != nil {
		return fmt.Errorf("start application: %w: %s", err, strings.TrimSpace(string(out)))
	}
	statusOK := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Error:") || strings.HasPrefix(line, "Error type") || strings.HasPrefix(line, "Exception") {
			return fmt.Errorf("start application failed: %s", strings.TrimSpace(string(out)))
		}
		if line == "Status: ok" {
			statusOK = true
		}
	}
	if !statusOK {
		return fmt.Errorf("start application returned no successful status: %s", strings.TrimSpace(string(out)))
	}

	verifyCtx, verifyCancel := context.WithTimeout(ctx, 5*time.Second)
	defer verifyCancel()
	var lastDetail string
	for {
		page, err := s.getUIPageProvider().FetchPageInfo(verifyCtx, serial)
		if err == nil && page != nil && page.PackageName == pkg {
			s.ResetPreviousScreen(serial)
			return nil
		}
		if err != nil {
			lastDetail = err.Error()
		} else if page != nil {
			lastDetail = "foreground package: " + page.PackageName
		} else {
			lastDetail = "foreground page unavailable"
		}
		select {
		case <-verifyCtx.Done():
			return fmt.Errorf("application start command completed but foreground %s was not verified: %s: %w", pkg, lastDetail, verifyCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}
