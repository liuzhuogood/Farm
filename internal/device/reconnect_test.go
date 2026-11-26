package device

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"farm/internal/db"
)

func newManualManager(t *testing.T, dir, adbBody string, autoReconnect int) (*Manager, *db.DB, string) {
	t.Helper()
	t.Setenv("FAKE_CHANNEL_DIR", dir)
	adb := fakeChannelADB(t, dir, adbBody)
	database, err := db.Open(filepath.Join(dir, "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.UpsertManualDevice("10.0.0.9:5555", "Desk", autoReconnect); err != nil {
		t.Fatal(err)
	}
	m := NewManager(adb, database)
	t.Cleanup(func() { m.Stop() })
	return m, database, filepath.Join(dir, "calls")
}

func readCalls(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(content)
}

// 传输僵死时 adb connect 只回 already connected；第二次尝试必须先 disconnect 再 connect。
func TestReconnectClearsStaleTransportBeforeRetry(t *testing.T) {
	dir := t.TempDir()
	m, _, calls := newManualManager(t, dir, `
case "$1" in
  devices) cat "$FAKE_CHANNEL_DIR/devices";;
  connect) printf 'connect %s\n' "$2" >> "$FAKE_CHANNEL_DIR/calls"; printf 'already connected to %s\n' "$2";;
  disconnect) printf 'disconnect %s\n' "$2" >> "$FAKE_CHANNEL_DIR/calls"; printf 'disconnected %s\n' "$2";;
esac
`, 1)
	writeChannelFixture(t, dir, "devices", "10.0.0.9:5555 offline\n")

	m.refresh()
	m.reconnectOfflineManualDevices()
	waitFor(t, func() bool { return strings.Contains(readCalls(t, calls), "connect 10.0.0.9:5555") }, "首次重连未执行")
	if got := readCalls(t, calls); strings.Contains(got, "disconnect") {
		t.Fatalf("首次重连不应先断开: %q", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.reconnectFailureCount("10.0.0.9:5555") < 1 {
		if time.Now().After(deadline) {
			t.Fatal("离线设备未记录重连失败")
		}
		time.Sleep(10 * time.Millisecond)
	}

	writeChannelFixture(t, dir, "devices", "10.0.0.9:5555 device\n")
	m.reconnectOfflineManualDevices()
	waitFor(t, func() bool { return m.deviceState("10.0.0.9:5555") == "device" }, "设备恢复后状态未刷新")
	got := readCalls(t, calls)
	if !strings.Contains(got, "disconnect 10.0.0.9:5555") {
		t.Fatalf("恢复前应先清理旧传输: %q", got)
	}
	if strings.Count(got, "connect 10.0.0.9:5555") < 2 {
		t.Fatalf("恢复前应重新 connect: %q", got)
	}
	if n := m.reconnectFailureCount("10.0.0.9:5555"); n != 0 {
		t.Fatalf("恢复后失败计数未清零: %d", n)
	}
}

// adb devices 读取失败时保留上一份快照，不能把在线设备误标为离线。
func TestRefreshRetainsSnapshotWhenAdbDevicesFails(t *testing.T) {
	dir := t.TempDir()
	m, _, _ := newManualManager(t, dir, `
if [ -f "$FAKE_CHANNEL_DIR/fail" ]; then
  printf 'error: cannot connect to daemon\n' >&2
  exit 1
fi
cat "$FAKE_CHANNEL_DIR/devices"
`, 1)
	writeChannelFixture(t, dir, "devices", "10.0.0.9:5555 device\n")
	m.refresh()
	if got := m.deviceState("10.0.0.9:5555"); got != "device" {
		t.Fatalf("初始状态 %q", got)
	}

	writeChannelFixture(t, dir, "fail", "")
	m.refresh()
	if got := m.deviceState("10.0.0.9:5555"); got != "device" {
		t.Fatalf("adb devices 失败时快照被改写: %q", got)
	}
	m.reconnectMu.Lock()
	fails := m.devicesFails
	m.reconnectMu.Unlock()
	if fails != 1 {
		t.Fatalf("连续失败计数 = %d", fails)
	}
	_ = os.Remove(filepath.Join(dir, "fail"))
	m.refresh()
	m.reconnectMu.Lock()
	fails = m.devicesFails
	m.reconnectMu.Unlock()
	if fails != 0 {
		t.Fatalf("恢复后失败计数未清零: %d", fails)
	}
}

// 所有设备离线且重连连续失败时自动重启 ADB 服务，并且只记录一次。
func TestEscalateRestartADBWhenNothingOnline(t *testing.T) {
	dir := t.TempDir()
	m, _, calls := newManualManager(t, dir, `
case "$1" in
  kill-server|start-server) printf '%s\n' "$1" >> "$FAKE_CHANNEL_DIR/calls";;
esac
`, 1)
	writeChannelFixture(t, dir, "devices", "")
	m.refresh()

	for i := 0; i < reconnectRestartAfter; i++ {
		m.recordReconnectFailure("10.0.0.9:5555")
	}
	m.reconnectOfflineManualDevices()
	waitFor(t, func() bool {
		got := readCalls(t, calls)
		return strings.Contains(got, "kill-server") && strings.Contains(got, "start-server")
	}, "未自动重启 ADB 服务")

	m.reconnectOfflineManualDevices()
	time.Sleep(100 * time.Millisecond)
	if got := strings.Count(readCalls(t, calls), "kill-server"); got != 1 {
		t.Fatalf("冷却窗口内重复重启 ADB: %d 次, calls=%q", got, readCalls(t, calls))
	}
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
