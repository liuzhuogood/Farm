package automation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"farm/internal/uipage"
)

func TestLaunchAppVerifiesForegroundAndClearsOldScreen(t *testing.T) {
	svc := &Service{}
	svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{PackageName: "com.android.settings"}})
	svc.screenCache.Store("device-1", &Screen{})
	var calls [][]string
	err := svc.launchAppWithRunner(context.Background(), "device-1", "com.android.settings", func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(calls) == 1 {
			return []byte("priority=0\ncom.android.settings/.Settings\n"), nil
		}
		return []byte("Starting: Intent {}\nStatus: ok\nComplete\n"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "device-1", "shell", "am", "start", "-W", "-n", "com.android.settings/.Settings"}
	if len(calls) != 2 || !reflect.DeepEqual(calls[1], want) {
		t.Fatalf("calls: %#v", calls)
	}
	if _, ok := svc.screenCache.Load("device-1"); ok {
		t.Fatal("old screen still cached after app launch")
	}
}

func TestLaunchAppRejectsAliasesAndCommandFailures(t *testing.T) {
	for _, tc := range []struct {
		name, pkg, resolve, start string
		commandErr                error
		want                      string
	}{
		{name: "alias", pkg: "settings", want: "explicit Android package"},
		{name: "injection", pkg: "com.android.settings;reboot", want: "explicit Android package"},
		{name: "no launcher", pkg: "com.android.settings", resolve: "No activity found", want: "no launcher activity"},
		{name: "resolve failure", pkg: "com.android.settings", resolve: "device offline", commandErr: errors.New("exit status 1"), want: "device offline"},
		{name: "am error with zero exit", pkg: "com.android.settings", resolve: "com.android.settings/.Settings", start: "Error type 3\nError: Activity does not exist", want: "start application failed"},
		{name: "no status", pkg: "com.android.settings", resolve: "com.android.settings/.Settings", start: "Starting: Intent {}", want: "no successful status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{}
			calls := 0
			err := svc.launchAppWithRunner(context.Background(), "d", tc.pkg, func(_ context.Context, _ ...string) ([]byte, error) {
				calls++
				if calls == 1 {
					return []byte(tc.resolve), tc.commandErr
				}
				return []byte(tc.start), nil
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if (tc.name == "alias" || tc.name == "injection") && calls != 0 {
				t.Fatalf("invalid package reached ADB")
			}
		})
	}
}

func TestLaunchAppDoesNotTreatCommandSuccessAsForegroundSuccess(t *testing.T) {
	svc := &Service{}
	svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{PackageName: "com.other.app"}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	calls := 0
	err := svc.launchAppWithRunner(ctx, "d", "com.android.settings", func(_ context.Context, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("com.android.settings/.Settings"), nil
		}
		return []byte("Status: ok"), nil
	})
	if err == nil || !strings.Contains(err.Error(), "foreground com.android.settings was not verified") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected result: %v", err)
	}
}
