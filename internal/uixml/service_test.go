package uixml

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchPreservesADBDiagnosticsAndDoesNotReadStaleXML(t *testing.T) {
	calls := 0
	s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		return nil, &exec.ExitError{Stderr: []byte("adb: device 'redmi' not found\n")}
	})
	_, _, err := s.Fetch(context.Background(), "redmi")
	if err == nil || !strings.Contains(err.Error(), "device 'redmi' not found") || calls != 2 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestFetchPreservesAndroidDumpOutput(t *testing.T) {
	s := NewWithRunner(func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("ERROR: could not get idle state."), errors.New("exit status 1")
	})
	_, _, err := s.Fetch(context.Background(), "device")
	if err == nil || !strings.Contains(err.Error(), "could not get idle state") {
		t.Fatalf("missing Android diagnostic: %v", err)
	}
}

func TestFetchReportsDeadline(t *testing.T) {
	s := NewWithRunner(func(ctx context.Context, args ...string) ([]byte, error) {
		if args[3] == "rm" {
			return nil, nil
		}
		<-ctx.Done()
		return nil, errors.New("signal: killed")
	})
	s.timeout = time.Millisecond
	_, _, err := s.Fetch(context.Background(), "device")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
}

func TestFetchRunsDumpThenRead(t *testing.T) {
	var calls [][]string
	s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(calls) == 1 {
			return []byte("UI dump written"), nil
		}
		return []byte(`<hierarchy><node text="ok"/></hierarchy>`), nil
	})
	x, captured, err := s.Fetch(context.Background(), "device-1")
	if err != nil || x == "" || captured.IsZero() {
		t.Fatalf("fetch: %q %v %v", x, captured, err)
	}
	if len(calls) != 4 || calls[0][3] != "uiautomator" || calls[1][2] != "exec-out" || calls[2][3] != "wm" {
		t.Fatalf("calls: %#v", calls)
	}
	if calls[0][5] != calls[1][4] || calls[0][5] != calls[3][5] || !strings.HasPrefix(calls[0][5], "/data/local/tmp/farm-ui-") {
		t.Fatalf("dump/read/cleanup do not share this request's unique path: %#v", calls)
	}
}

func TestParseDisplaySizePrefersOverride(t *testing.T) {
	for _, tc := range []struct {
		output string
		want   *DisplaySize
	}{
		{"Physical size: 1080x2400\n", &DisplaySize{Width: 1080, Height: 2400}},
		{"Physical size: 1080x2400\nOverride size: 720x1600\n", &DisplaySize{Width: 720, Height: 1600}},
		{"unavailable", nil},
	} {
		if got := parseDisplaySize(tc.output); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("parseDisplaySize(%q) = %#v, want %#v", tc.output, got, tc.want)
		}
	}
}

func TestFetchRejectsInvalidAndTooLargeXML(t *testing.T) {
	for _, input := range []string{"<a/><b/>", "<a>", "junk<a/>", "<a/>junk"} {
		s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
			if args[2] == "shell" {
				return nil, nil
			}
			return []byte(input), nil
		})
		if _, _, err := s.Fetch(context.Background(), "d"); !errors.Is(err, ErrInvalidXML) {
			t.Errorf("input %q: got %v", input, err)
		}
	}
	s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
		if args[2] == "shell" {
			return nil, nil
		}
		return []byte(`<a/>`), nil
	})
	s.max = 2
	if _, _, err := s.Fetch(context.Background(), "d"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large: %v", err)
	}
}

func TestFetchSerializesConcurrentDumps(t *testing.T) {
	var mu sync.Mutex
	active, maxActive := 0, 0
	s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		if args[2] == "shell" {
			return nil, nil
		}
		return []byte(`<a/>`), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _ = s.Fetch(context.Background(), "d") }()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("max concurrent commands = %d", maxActive)
	}
}

func TestDumpExitZeroCannotReturnPreviousSnapshots(t *testing.T) {
	var dumpPaths []string
	s := NewWithRunner(func(_ context.Context, args ...string) ([]byte, error) {
		if args[2] == "shell" && args[3] == "uiautomator" {
			dumpPaths = append(dumpPaths, args[5])
			return []byte("ERROR: null root node"), nil
		}
		if args[2] == "exec-out" {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	})
	for i := 0; i < 2; i++ {
		if _, err := s.FetchSnapshot(context.Background(), "phone"); err == nil {
			t.Fatal("zero exit without a new snapshot was accepted")
		}
	}
	if len(dumpPaths) != 2 || dumpPaths[0] == dumpPaths[1] {
		t.Fatalf("paths reused across requests: %v", dumpPaths)
	}
}
