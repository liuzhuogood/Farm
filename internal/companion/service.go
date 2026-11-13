// Package companion connects to Farm's explicitly enabled, local-only Android
// AccessibilityService. It never installs or enables an agent during observation.
package companion

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"farm/internal/dependencies"
	"farm/internal/uixml"
)

const (
	Package          = "app.farm.companion"
	Component        = Package + "/.FarmAccessibilityService"
	protocolVersion  = 2
	maxResponseBytes = 4 << 20
	snapshotTimeout  = 4 * time.Second
	pingTimeout      = 2 * time.Second
	restoreTimeout   = 8 * time.Second
)

type runner func(context.Context, ...string) ([]byte, error)
type deviceGate struct {
	token    chan struct{}
	users    int
	revision uint64
}
type Service struct {
	run   runner
	dial  func(context.Context, string, string) (net.Conn, error)
	apk   []byte
	mu    sync.Mutex
	gates map[string]*deviceGate
	slots chan struct{}
}

func New(adbPath string, apk []byte) *Service {
	return &Service{run: func(ctx context.Context, args ...string) ([]byte, error) {
		out, err := dependencies.CommandContext(ctx, adbPath, args...).Output()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return out, err
	}, dial: (&net.Dialer{}).DialContext, apk: apk, gates: make(map[string]*deviceGate), slots: make(chan struct{}, 8)}
}

// acquire is context-cancellable. Gates are reclaimed after the last waiter.
// Explicit mutations increment revision when queued, before taking the gate.
func (s *Service) acquire(ctx context.Context, serial string, mutation bool) (*deviceGate, func(), error) {
	if strings.TrimSpace(serial) == "" {
		return nil, nil, uixml.ErrMissingSerial
	}
	s.mu.Lock()
	g := s.gates[serial]
	if g == nil {
		g = &deviceGate{token: make(chan struct{}, 1)}
		s.gates[serial] = g
	}
	g.users++
	if mutation {
		g.revision++
	}
	s.mu.Unlock()
	drop := func() {
		s.mu.Lock()
		g.users--
		if g.users == 0 {
			delete(s.gates, serial)
		}
		s.mu.Unlock()
	}
	select {
	case g.token <- struct{}{}:
	case <-ctx.Done():
		drop()
		return nil, nil, ctx.Err()
	}
	return g, func() { <-g.token; drop() }, nil
}

func (s *Service) revision(g *deviceGate) uint64 { s.mu.Lock(); defer s.mu.Unlock(); return g.revision }

func (s *Service) command(ctx context.Context, serial string, args ...string) ([]byte, error) {
	out, err := s.run(ctx, append([]string{"-s", serial}, args...)...)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		detail := strings.TrimSpace(string(out))
		if len(detail) > 2048 {
			detail = detail[:2048]
		}
		return nil, fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, detail)
	}
	return out, nil
}

func (s *Service) FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	_, release, err := s.acquire(ctx, serial, false)
	if err != nil {
		return uixml.Snapshot{}, err
	}
	defer release()
	return s.fetchLocked(ctx, serial)
}

func (s *Service) fetchLocked(ctx context.Context, serial string) (snapshot uixml.Snapshot, resultErr error) {
	started := time.Now()
	defer func() {
		log.Printf("[Companion] snapshot serial=%s elapsed_ms=%d xml_bytes=%d error=%v", serial, time.Since(started).Milliseconds(), len(snapshot.XML), resultErr)
	}()
	enabled, err := s.enabledServices(ctx, serial)
	if err != nil {
		return snapshot, err
	}
	master, err := s.accessibilityMaster(ctx, serial)
	if err != nil {
		return snapshot, err
	}
	if !containsService(enabled) || master != "1" {
		return snapshot, fmt.Errorf("companion accessibility service is not enabled")
	}
	response, err := s.exchange(ctx, serial, "snapshot")
	if err != nil {
		return snapshot, err
	}
	if len(response.XML) > 2<<20 {
		return snapshot, fmt.Errorf("companion XML exceeds 2 MiB")
	}
	if response.Width <= 0 || response.Height <= 0 || response.CapturedAtMS <= 0 {
		return snapshot, fmt.Errorf("invalid companion snapshot metadata")
	}
	if err := validateHierarchy(response.XML); err != nil {
		return snapshot, err
	}
	snapshot = uixml.Snapshot{XML: response.XML, CapturedAt: time.UnixMilli(response.CapturedAtMS).UTC(), DisplaySize: &uixml.DisplaySize{Width: response.Width, Height: response.Height}, Source: "companion"}
	return snapshot, nil
}

type wireResponse struct {
	Version      int    `json:"version"`
	Command      string `json:"command"`
	XML          string `json:"xml"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	CapturedAtMS int64  `json:"captured_at_ms"`
	Error        string `json:"error"`
}

type unsupportedProtocolError struct{ detail string }

func (e unsupportedProtocolError) Error() string { return e.detail + "; update the Farm Companion APK" }

// exchange shares bounded framing, transport, capacity and forward cleanup for
// snapshot and ping. A ping never invokes the Android tree capture executor.
func (s *Service) exchange(ctx context.Context, serial, command string) (response wireResponse, resultErr error) {
	started := time.Now()
	defer func() {
		log.Printf("[Companion] exchange serial=%s command=%s elapsed_ms=%d error=%v", serial, command, time.Since(started).Milliseconds(), resultErr)
	}()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return response, fmt.Errorf("companion capacity wait: %w", ctx.Err())
	}
	out, err := s.command(ctx, serial, "forward", "tcp:0", "localabstract:farm_companion")
	if err != nil {
		return response, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || port < 1 || port > 65535 {
		return response, fmt.Errorf("invalid adb forward port %q", strings.TrimSpace(string(out)))
	}
	local := "tcp:" + strconv.Itoa(port)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := s.command(cleanupCtx, serial, "forward", "--remove", local); err != nil {
			log.Printf("[Companion] remove forward failed serial=%s local=%s error=%v", serial, local, err)
		}
	}()
	conn, err := s.dial(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return response, fmt.Errorf("connect companion: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := json.NewEncoder(conn).Encode(struct {
		Version int    `json:"version"`
		Command string `json:"command"`
	}{protocolVersion, command}); err != nil {
		return response, fmt.Errorf("send %s request: %w", command, err)
	}
	line, err := bufio.NewReader(io.LimitReader(conn, maxResponseBytes+1)).ReadBytes('\n')
	if len(line) > maxResponseBytes {
		return response, fmt.Errorf("companion response exceeds %d bytes", maxResponseBytes)
	}
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return response, fmt.Errorf("read %s response: %w", command, err)
	}

	if err := json.Unmarshal(line, &response); err != nil {
		return response, fmt.Errorf("decode companion response: %w", err)
	}
	if response.Version != protocolVersion {
		return response, unsupportedProtocolError{detail: fmt.Sprintf("unsupported companion protocol version %d (expected %d)", response.Version, protocolVersion)}
	}
	if response.Error != "" {
		return response, agentResponseError{message: response.Error}
	}

	return response, nil
}

func (s *Service) pingLocked(ctx context.Context, serial string) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	response, err := s.exchange(ctx, serial, "ping")
	if err != nil {
		return err
	}
	if response.Command != "pong" {
		return unsupportedProtocolError{detail: fmt.Sprintf("unexpected companion ping response command %q", response.Command)}
	}
	return nil
}

// waitReadyLocked tolerates service restart and transient ADB transport errors
// only within its caller's budget. Protocol incompatibility requires an update.
func (s *Service) waitReadyLocked(ctx context.Context, serial string) error {
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, lastErr)
		}
		lastErr = s.pingLocked(ctx, serial)
		if lastErr == nil {
			return nil
		}
		var protocolErr unsupportedProtocolError
		if errors.As(lastErr, &protocolErr) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func validateHierarchy(value string) error {
	dec := xml.NewDecoder(strings.NewReader(value))
	depth := 0
	roots := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if roots != 1 || depth != 0 {
				return fmt.Errorf("invalid companion hierarchy")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid companion XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || t.Name.Local != "hierarchy" {
					return fmt.Errorf("invalid companion hierarchy root")
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return fmt.Errorf("invalid companion XML outside hierarchy")
			}
		}
	}
}

func containsService(services string) bool {
	for _, v := range strings.Split(services, ":") {
		if v == Component || v == Package+"/"+Package+".FarmAccessibilityService" {
			return true
		}
	}
	return false
}
func (s *Service) enabledServices(ctx context.Context, serial string) (string, error) {
	out, err := s.command(ctx, serial, "shell", "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out))
	if value == "null" {
		value = ""
	}
	return value, nil
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

// A valid protocol error proves that the service is reachable even when the
// current window cannot provide a usable tree and dump fallback is needed.
type agentResponseError struct{ message string }

func (e agentResponseError) Error() string { return "companion: " + e.message }
func (s *Service) accessibilityMaster(ctx context.Context, serial string) (string, error) {
	out, err := s.command(ctx, serial, "shell", "settings", "get", "secure", "accessibility_enabled")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
