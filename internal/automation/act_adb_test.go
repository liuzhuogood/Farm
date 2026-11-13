package automation

import (
	"farm/internal/device"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestActADBQuotedArgumentsAndTargetValidation(t *testing.T) {
	args, err := parseActADB(`shell echo "hello world" '中文 单引号' empty\ value ""`)
	want := []string{"shell", "echo", "hello world", "中文 单引号", "empty value", ""}
	if err != nil || !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q err=%v", args, err)
	}
	for _, input := range []string{"", "adb shell id", "-s other shell id", "--serial=other shell id", "-Hhost shell id", "-t3 shell id", "shell echo 'oops", "shell echo \\"} {
		if _, err := parseActADB(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	for _, input := range []string{"shell ls -t /sdcard", "shell pm list packages -s", "shell input text '-Hello'"} {
		if _, err := parseActADB(input); err != nil {
			t.Errorf("rejected remote option %q: %v", input, err)
		}
	}
}

func TestActADBReportsOutputAndStopsBatchOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "adb")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\ncase \"$3\" in fail) echo device-error >&2; exit 7;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	svc, transport := newActTestService()
	svc.adbPath = helper
	w := callAct(svc, "adb shell echo 'hello world'\nkeyevent back")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "-s\nphone\nshell\n'echo' 'hello world'") || len(transport.requests) != 1 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	w = callAct(svc, "adb fail\nkeyevent home")
	if w.Code != 502 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "device-error") || !strings.Contains(w.Body.String(), "失败行 1") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestActArtifactBatchPrevalidatesRelativeDeviceDirectories(t *testing.T) {
	svc, transport := newActTestService()
	providerCalls := 0
	svc.SetDeviceProvider(func() []device.Device { providerCalls++; return nil })
	for _, input := range []string{"mkdir /tmp/output", "mkdir ../escape", "screenshot task/../escape", "record 0 task-output", "record 3 /sdcard/output", "mkdir task-output\nrecord 31 task-output"} {
		w := callAct(svc, input)
		if w.Code != 400 || providerCalls != 0 || len(transport.requests) != 0 {
			t.Fatalf("invalid batch touched device: %q status=%d calls=%d body=%s", input, w.Code, providerCalls, w.Body.String())
		}
	}
	steps, _, err := parseActBatch("mkdir 产物 with spaces\nscreenshot 产物 with spaces\nrecord 3 产物 with spaces", "phone")
	if err != nil || len(steps) != 3 {
		t.Fatalf("parse=%+v err=%v", steps, err)
	}
	for _, step := range steps {
		if step.plan.directory != "产物 with spaces" {
			t.Fatalf("directory lost: %q", step.plan.directory)
		}
	}
}

func TestActADBOutputIsBounded(t *testing.T) {
	b := &actADBOutput{}
	input := strings.Repeat("x", 2<<20)
	if n, err := b.Write([]byte(input)); n != len(input) || err != nil {
		t.Fatal(n, err)
	}
	if b.text.Len() != 1<<20 || !b.truncated {
		t.Fatal("output limit not enforced")
	}
}

func TestActADBShellOptionsAndRemoteQuoting(t *testing.T) {
	args, err := parseActADB(`shell -T -e none printf '%s' "hello world" "it's literal; $(unchanged)"`)
	if err != nil {
		t.Fatal(err)
	}
	got := actADBCommandArgs(args)
	want := []string{"shell", "-T", "-e", "none", `'printf' '%s' 'hello world' 'it'"'"'s literal; $(unchanged)'`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%q want=%q", got, want)
	}
}
