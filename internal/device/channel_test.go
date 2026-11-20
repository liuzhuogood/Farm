package device

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"farm/internal/db"
)

func TestValidIdentityValue(t *testing.T) {
	for _, value := range []string{"", " \t\r\n", "unknown", "UnKnOwN", "null", "0", "0000", "0123456789ABCDEF", "0123456789abcdef", "123", strings.Repeat("x", 65), "multi\nline"} {
		if got := validIdentityValue(value); got != "" {
			t.Errorf("%q accepted as %q", value, got)
		}
	}
	for _, value := range []string{"a1c2", strings.Repeat("X", 64)} {
		if got := validIdentityValue(" \t" + value + "\r\n"); got != value {
			t.Errorf("got %q, want %q", got, value)
		}
	}
}

func TestReadChannelIdentitySourceOrder(t *testing.T) {
	for _, tc := range []struct {
		name, commands, wantID, wantSource, wantCalls string
	}{
		{"primary", `printf ' Primary123 \r\n';;`, "hw:Primary123", "ro.serialno", "ro.serialno\n"},
		{"boot", `printf 'UNKNOWN\n';;
ro.boot.serialno) printf 'Boot123\n';;`, "hw:Boot123", "ro.boot.serialno", "ro.serialno\nro.boot.serialno\n"},
		{"android", `printf 'BadButExitFailure\n'; exit 1;;
ro.boot.serialno) printf '000000\n';;
android_id) printf '1234abcdef\n';;`, "aid:1234abcdef", "android_id", "ro.serialno\nro.boot.serialno\nandroid_id\n"},
		{"pending", `printf '0123456789ABCDEF\n';;
ro.boot.serialno) printf 'null\n';;
android_id) exit 1;;`, "", "pending", "ro.serialno\nro.boot.serialno\nandroid_id\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			calls := filepath.Join(dir, "calls")
			t.Setenv("FAKE_CALLS", calls)
			adb := fakeChannelADB(t, dir, `
[ "$1 $2 $3" = '-s test-serial shell' ] || exit 1
case "$4 $5 $6 $7" in
 "getprop ro.serialno  "|"getprop ro.boot.serialno  ") source="$5";;
 "settings get secure android_id") source="$7";;
 *) exit 1;;
esac
printf '%s\n' "$source" >> "$FAKE_CALLS"
case "$source" in
ro.serialno) `+tc.commands+"\nesac\n")
			m := NewManager(adb, nil)
			defer m.Stop()
			if id, source := m.readChannelIdentity(context.Background(), "test-serial"); id != tc.wantID || source != tc.wantSource {
				t.Fatalf("identity=%q source=%q, want %q %q", id, source, tc.wantID, tc.wantSource)
			}
			content, err := os.ReadFile(calls)
			if err != nil || string(content) != tc.wantCalls {
				t.Fatalf("ADB calls = %q, want %q (error: %v)", content, tc.wantCalls, err)
			}
		})
	}
}

func TestRefreshResamplesChannelIdentityOnReconnect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FAKE_CHANNEL_DIR", dir)
	adb := fakeChannelADB(t, dir, `
case "$1 $3 $4 $5" in
 "devices   ") cat "$FAKE_CHANNEL_DIR/devices";;
 "connect   ") printf 'connected to 192.0.2.2:5555\n';;
 "-s shell getprop ro.serialno")
   printf '%s\n' "$2" >> "$FAKE_CHANNEL_DIR/calls"
   cat "$FAKE_CHANNEL_DIR/identity";;
esac
`)
	writeChannelFixture(t, dir, "identity", "Phone123\n")
	const usb = "USB123"
	const wifi = "192.0.2.2:5555"
	const both = "USB123 device usb:1-1 transport_id:1\n192.0.2.2:5555 device transport_id:2\n"
	writeChannelFixture(t, dir, "devices", both+"NEW123 unauthorized\n")

	m := NewManager(adb, nil)
	defer m.Stop()
	m.refresh()
	m.identityProbes.Wait()

	devices := m.Devices()
	if len(devices) != 2 {
		t.Fatalf("want one merged device plus one pending row, got %+v", devices)
	}
	if d := devices[0]; d.Identity != "hw:Phone123" || d.IdentitySource != "ro.serialno" ||
		d.ActiveKind != "usb" || len(d.Connections) != 2 ||
		d.Connections[0].Kind != "usb" || d.Connections[1].Kind != "wifi" {
		t.Fatalf("merged row %+v", d)
	}
	if d := devices[1]; d.Identity != "" || d.IdentitySource != "pending" || d.ActiveKind != "" ||
		len(d.Connections) != 1 || d.Connections[0].State != "unauthorized" || d.Connections[0].Online {
		t.Fatalf("unauthorized row %+v", d)
	}
	devices[0].Connections[0].Serial = "mutated snapshot"
	if m.Devices()[0].Connections[0].Serial != usb {
		t.Fatal("returned connection aliases manager state")
	}
	m.refresh()
	m.identityProbes.Wait()
	assertChannelCalls(t, dir, 2)

	// An unplugged USB keeps its identity for grouping: the card stays merged
	// and the current channel moves to wifi.
	writeChannelFixture(t, dir, "devices", "192.0.2.2:5555 device transport_id:2\n")
	m.refresh()
	if d := m.Devices()[0]; d.Serial != wifi || d.ActiveKind != "wifi" || d.Identity != "hw:Phone123" ||
		d.Connections[0].Kind != "usb" || d.Connections[0].Online ||
		d.Connections[1].Kind != "wifi" || !d.Connections[1].Online || !d.Connections[1].Active {
		t.Fatalf("failover row %+v", d)
	}
	assertChannelCalls(t, dir, 2)

	// Replugging replays identity collection on the USB channel.
	writeChannelFixture(t, dir, "identity", "Phone456\n")
	writeChannelFixture(t, dir, "devices", both)
	m.refresh()
	m.identityProbes.Wait()
	assertChannelCalls(t, dir, 3)
	if got := m.channels[usb].identity; got != "hw:Phone456" {
		t.Fatalf("USB identity after replug: %q", got)
	}
	if got := m.channels[wifi].identity; got != "hw:Phone123" {
		t.Fatalf("wifi identity was overwritten by the USB channel: %q", got)
	}

	// A successful reconnect resamples the wifi channel.
	writeChannelFixture(t, dir, "identity", "Phone789\n")
	if err := m.Connect("192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	m.identityProbes.Wait()
	assertChannelCalls(t, dir, 4)
	if got := m.channels[wifi].identity; got != "hw:Phone789" {
		t.Fatalf("wifi identity after successful Connect: %q", got)
	}

	// A new transport id may mean the address now belongs to another phone.
	writeChannelFixture(t, dir, "identity", "Phone999\n")
	writeChannelFixture(t, dir, "devices", strings.Replace(both, "transport_id:2", "transport_id:3", 1))
	m.refresh()
	m.identityProbes.Wait()
	assertChannelCalls(t, dir, 5)
	if got := m.channels[wifi].identity; got != "hw:Phone999" {
		t.Fatalf("wifi identity after transport change: %q", got)
	}
}

func TestRefreshDoesNotProbeStoredOnlineState(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	serial := "192.0.2.3:5555"
	if _, err := database.UpsertManualDevice(serial, "Desk", 0); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateDeviceStatus(serial, "device", "", "", "", "", true); err != nil {
		t.Fatal(err)
	}
	adb := fakeChannelADB(t, dir, `[ "$1" = devices ]`)
	m := NewManager(adb, database)
	defer m.Stop()
	m.refresh()
	m.identityProbes.Wait()
	d := m.Devices()[0]
	if d.State != "offline" || d.Alias != "Desk" || d.IdentitySource != "pending" || d.ActiveKind != "" ||
		len(d.Connections) != 1 || d.Connections[0].State != "offline" || d.Connections[0].Online || d.Connections[0].LastSeenAt != 0 {
		t.Fatalf("legacy state must not make a channel online or the device online: %+v", d)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.channels[serial].probe != nil {
		t.Fatal("probed a serial that was absent from adb devices")
	}
}

func TestRefreshDoesNotWaitForProbeOrPublishStaleIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FAKE_CHANNEL_DIR", dir)
	adb := fakeChannelADB(t, dir, `
case "$1 $3 $4 $5" in
 "devices   ") cat "$FAKE_CHANNEL_DIR/devices";;
 "-s shell getprop ro.serialno")
   if [ "$2" = phone ]; then
     : > "$FAKE_CHANNEL_DIR/probe-started"
     exec sleep 30
   fi
   printf 'Other123\n';;
esac
`)
	writeChannelFixture(t, dir, "devices", "phone device\nother device\n")
	m := NewManager(adb, nil)
	defer m.Stop()
	m.hardwareCache["phone"] = HardwareInfo{}
	m.hardwareCache["other"] = HardwareInfo{}
	m.refresh()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := os.Stat(filepath.Join(dir, "probe-started"))
		if err == nil && m.Devices()[1].Identity == "hw:Other123" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a stalled probe prevented another channel from completing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if d := m.Devices()[0]; d.ActiveKind != "usb" || d.IdentitySource != "pending" {
		t.Fatalf("stalled probe should leave an online pending row: %+v", d)
	}
	writeChannelFixture(t, dir, "devices", "phone offline\n")
	m.refresh()
	m.identityProbes.Wait()
	if d := m.Devices()[0]; d.ActiveKind != "" || d.Identity != "" || d.IdentitySource != "pending" {
		t.Fatalf("unexpected offline identity: %+v", d)
	}
}

func TestChannelConnectionKinds(t *testing.T) {
	for _, tc := range []struct{ serial, source, line, kind, address string }{
		{"phone", "auto", "phone device usb:1-1", "usb", ""},
		{"adb-phone-Gnqt77._adb-tls-connect._tcp.", "auto", "adb-phone-Gnqt77._adb-tls-connect._tcp. device", "wifi", ""},
		{"adb-phone-Gnqt77", "auto", "adb-phone-Gnqt77 device", "wifi", ""},
		{"[2001:db8::1]:5555", "auto", "[2001:db8::1]:5555 device", "wifi", "[2001:db8::1]:5555"},
		{"192.0.2.2", "manual", "192.0.2.2 device", "wifi", "192.0.2.2"},
		{"farm-phone.local", "manual", "", "wifi", "farm-phone.local"},
	} {
		t.Run(tc.serial, func(t *testing.T) {
			var observed Device
			if rows := parseDevices(tc.line); len(rows) > 0 {
				observed = rows[0]
			}
			got := channelConnection(Device{Serial: tc.serial, Source: tc.source}, observed)
			if got.Kind != tc.kind || got.Address != tc.address {
				t.Fatalf("connection %+v, want %q %q", got, tc.kind, tc.address)
			}
		})
	}
}

func fakeChannelADB(t *testing.T, dir, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell executable")
	}
	return writeChannelFixture(t, dir, "adb", "#!/bin/sh\n"+body)
}

func writeChannelFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertChannelCalls(t *testing.T, dir string, want int) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "calls"))
	if count := strings.Count(string(content), "\n"); err != nil || count != want {
		t.Fatalf("ADB probes: got %d, want %d, calls=%q, error=%v", count, want, content, err)
	}
}
