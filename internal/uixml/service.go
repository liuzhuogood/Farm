// Package uixml provides a bounded, read-only Android UI XML snapshot service.
package uixml

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"farm/internal/dependencies"
	"fmt"
	"io"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxBytes = 2 << 20
	defaultTimeout  = 15 * time.Second
)

var (
	ErrMissingSerial = errors.New("missing serial")
	ErrInvalidXML    = errors.New("invalid UI XML")
	ErrTooLarge      = errors.New("UI XML exceeds size limit")
)

// Runner executes one adb invocation. It is injectable so command behavior can
// be tested without a connected device.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// DisplaySize is the device coordinate space used by Android UI bounds.
// It is optional because an XML snapshot remains useful if wm size fails.
type DisplaySize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Snapshot struct {
	XML         string       `json:"xml"`
	Source      string       `json:"source,omitempty"`
	CapturedAt  time.Time    `json:"captured_at"`
	DisplaySize *DisplaySize `json:"display_size,omitempty"`
}

// Service takes a point-in-time UI hierarchy snapshot. A device can only run
// one UIAutomator session at a time. Each request uses its own temporary file.
type Service struct {
	run     Runner
	max     int64
	timeout time.Duration
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
}

func New(adbPath string) *Service {
	return NewWithRunner(func(ctx context.Context, args ...string) ([]byte, error) {
		return dependencies.CommandContext(ctx, adbPath, args...).Output()
	})
}

func NewWithRunner(run Runner) *Service {
	return &Service{run: run, max: DefaultMaxBytes, timeout: defaultTimeout, locks: make(map[string]*sync.Mutex)}
}

func (s *Service) lockFor(serial string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[serial] == nil {
		s.locks[serial] = &sync.Mutex{}
	}
	return s.locks[serial]
}

// Fetch returns the raw, validated XML and capture time.
func (s *Service) Fetch(ctx context.Context, serial string) (string, time.Time, error) {
	snapshot, err := s.FetchSnapshot(ctx, serial)
	if err != nil {
		return "", time.Time{}, err
	}
	return snapshot.XML, snapshot.CapturedAt, nil
}

// FetchSnapshot also reads the effective Android display size when available.
// UI Automator can omit system insets from maximum node bounds while the video
// stream is based on the full display coordinate space.
func (s *Service) FetchSnapshot(ctx context.Context, serial string) (snapshot Snapshot, fetchErr error) {
	if strings.TrimSpace(serial) == "" {
		return Snapshot{}, ErrMissingSerial
	}
	lock := s.lockFor(serial)
	lock.Lock()
	defer lock.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	started := time.Now()
	defer func() {
		log.Printf("[UIXML] snapshot serial=%s elapsed_ms=%d xml_bytes=%d error=%v", serial, time.Since(started).Milliseconds(), len(snapshot.XML), fetchErr)
	}()
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Snapshot{}, fmt.Errorf("create UI XML snapshot path: %w", err)
	}
	dumpPath := "/data/local/tmp/farm-ui-" + hex.EncodeToString(nonce[:]) + ".xml"
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		if _, err := s.run(cleanupCtx, "-s", serial, "shell", "rm", "-f", dumpPath); err != nil {
			log.Printf("[UIXML] temporary file cleanup failed serial=%s error=%v", serial, err)
		}
	}()
	if out, err := s.run(ctx, "-s", serial, "shell", "uiautomator", "dump", dumpPath); err != nil {
		return Snapshot{}, commandError(ctx, "uiautomator dump", out, err)
	}
	// Some Android builds print an error but exit zero without writing a file.
	// A unique path prevents a previous screen's valid XML becoming fake success.
	out, err := s.run(ctx, "-s", serial, "exec-out", "cat", dumpPath)
	if err != nil {
		return Snapshot{}, commandError(ctx, "read UI XML", out, err)
	}
	if int64(len(out)) > s.max {
		return Snapshot{}, ErrTooLarge
	}
	xmlText := string(out)
	if err := validateXML(strings.NewReader(xmlText)); err != nil {
		return Snapshot{}, err
	}
	snapshot = Snapshot{XML: xmlText, Source: "uiautomator", CapturedAt: time.Now().UTC()}
	if out, err := s.run(ctx, "-s", serial, "shell", "wm", "size"); err == nil {
		snapshot.DisplaySize = parseDisplaySize(string(out))
	}
	return snapshot, nil
}

// Output keeps stderr separate from successful XML, but retains Android/ADB
// diagnostics on failure rather than reducing every error to "exit status 1".
func commandError(ctx context.Context, stage string, out []byte, err error) error {
	var exitErr *exec.ExitError
	detail := strings.TrimSpace(string(out))
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		detail = strings.TrimSpace(detail + "\n" + string(exitErr.Stderr))
	}
	if len(detail) > 4096 {
		detail = detail[:4096] + "…"
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if detail != "" {
		return fmt.Errorf("%s: %w: %s", stage, err, detail)
	}
	return fmt.Errorf("%s: %w", stage, err)
}

var displaySizePattern = regexp.MustCompile(`(?m)^(Physical size|Override size):\s*(\d+)x(\d+)\s*$`)

func parseDisplaySize(output string) *DisplaySize {
	var physical, override *DisplaySize
	for _, match := range displaySizePattern.FindAllStringSubmatch(output, -1) {
		width, _ := strconv.Atoi(match[2])
		height, _ := strconv.Atoi(match[3])
		if width <= 0 || height <= 0 {
			continue
		}
		size := &DisplaySize{Width: width, Height: height}
		if match[1] == "Override size" {
			override = size
		} else {
			physical = size
		}
	}
	if override != nil {
		return override
	}
	return physical
}

func validateXML(r io.Reader) error {
	dec := xml.NewDecoder(r)
	depth, roots := 0, 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if roots != 1 || depth != 0 {
				return ErrInvalidXML
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidXML, err)
		}
		switch tok.(type) {
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(tok.(xml.CharData))) != "" {
				return ErrInvalidXML
			}
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return ErrInvalidXML
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				return ErrInvalidXML
			}
		}
	}
}
