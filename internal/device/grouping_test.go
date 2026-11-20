package device

import (
	"path/filepath"
	"testing"

	"farm/internal/db"
)

const identityFixtureADB = `
case "$1 $3 $4 $5" in
 "devices   ") cat "$FAKE_CHANNEL_DIR/devices";;
 "-s shell getprop ro.serialno")
   file="$FAKE_CHANNEL_DIR/id-$2"
   [ -f "$file" ] && cat "$file";;
esac
`

func newGroupingManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FAKE_CHANNEL_DIR", dir)
	database, err := db.Open(filepath.Join(dir, "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	m := NewManager(fakeChannelADB(t, dir, identityFixtureADB), database)
	t.Cleanup(m.Stop)
	return m, dir
}

func TestGroupedSnapshotMergesDualChannels(t *testing.T) {
	m, dir := newGroupingManager(t)
	const usb = "USB123"
	const wifi = "192.0.2.2:5555"
	writeChannelFixture(t, dir, "id-"+usb, "Phone123\n")
	writeChannelFixture(t, dir, "id-"+wifi, "Phone123\n")
	writeChannelFixture(t, dir, "devices", "USB123 device usb:1-1 transport_id:1\n192.0.2.2:5555 device transport_id:2\n")

	m.refresh()
	m.identityProbes.Wait()

	devices := m.Devices()
	if len(devices) != 1 {
		t.Fatalf("dual channels must merge into one entry, got %+v", devices)
	}
	merged := devices[0]
	if merged.Serial != usb || merged.Identity != "hw:Phone123" || merged.IdentitySource != "ro.serialno" ||
		merged.ActiveKind != "usb" || merged.State != "device" || len(merged.Connections) != 2 {
		t.Fatalf("merged snapshot %+v", merged)
	}
	if c := merged.Connections[0]; c.Kind != "usb" || !c.Online || !c.Active {
		t.Fatalf("usb connection %+v", c)
	}
	if c := merged.Connections[1]; c.Kind != "wifi" || !c.Online || c.Active || c.Address != wifi {
		t.Fatalf("wifi connection %+v", c)
	}

	if err := m.UpdateAlias(usb, "书桌手机"); err != nil {
		t.Fatal(err)
	}

	// Unplugging USB keeps one entry: the alias and order survive the channel
	// switch and the serial becomes the wifi one.
	writeChannelFixture(t, dir, "devices", "192.0.2.2:5555 device transport_id:2\n")
	m.refresh()
	devices = m.Devices()
	if len(devices) != 1 {
		t.Fatalf("unplugging USB must not add or drop entries, got %+v", devices)
	}
	switched := devices[0]
	if switched.Serial != wifi || switched.ActiveKind != "wifi" || switched.Alias != "书桌手机" ||
		switched.State != "device" || len(switched.Connections) != 2 {
		t.Fatalf("failover snapshot %+v", switched)
	}
	if c := switched.Connections[0]; c.Kind != "usb" || c.Online || c.Active {
		t.Fatalf("usb connection after unplug %+v", c)
	}
	if c := switched.Connections[1]; c.Kind != "wifi" || !c.Online || !c.Active {
		t.Fatalf("wifi connection after unplug %+v", c)
	}

	// The wifi address still resolves to the same device (S4 uses this).
	if byWifi, ok := m.GetDevice(wifi); !ok || byWifi.Identity != "hw:Phone123" {
		t.Fatalf("wifi serial lookup %+v ok=%t", byWifi, ok)
	}
}

func TestGroupedSnapshotKeepsDistinctIdentitiesApart(t *testing.T) {
	m, dir := newGroupingManager(t)
	writeChannelFixture(t, dir, "id-USB123", "PhoneA\n")
	writeChannelFixture(t, dir, "id-192.0.2.2:5555", "PhoneB\n")
	writeChannelFixture(t, dir, "devices", "USB123 device usb:1-1 transport_id:1\n192.0.2.2:5555 device transport_id:2\n")

	m.refresh()
	m.identityProbes.Wait()

	devices := m.Devices()
	if len(devices) != 2 {
		t.Fatalf("different identities must stay apart, got %+v", devices)
	}
	if devices[0].Identity != "hw:PhoneA" || devices[1].Identity != "hw:PhoneB" {
		t.Fatalf("unexpected identities %+v", devices)
	}
	for _, d := range devices {
		if len(d.Connections) != 1 || !d.Connections[0].Active {
			t.Fatalf("single channel device must stay single %+v", d)
		}
	}
}

func TestGroupedSnapshotCollapsesDuplicateKind(t *testing.T) {
	m, dir := newGroupingManager(t)
	for _, serial := range []string{"USB123", "192.0.2.2:5555", "adb-USB123-xyz"} {
		writeChannelFixture(t, dir, "id-"+serial, "Phone123\n")
	}
	writeChannelFixture(t, dir, "devices",
		"USB123 device usb:1-1 transport_id:1\n192.0.2.2:5555 device transport_id:2\nadb-USB123-xyz device transport_id:3\n")

	m.refresh()
	m.identityProbes.Wait()

	devices := m.Devices()
	if len(devices) != 1 {
		t.Fatalf("same identity must merge, got %+v", devices)
	}
	merged := devices[0]
	if len(merged.Connections) != 2 || merged.Connections[0].Kind != "usb" || merged.Connections[1].Kind != "wifi" {
		t.Fatalf("duplicate wifi channels must collapse to one per kind: %+v", merged.Connections)
	}
}

func TestGroupedSnapshotMergesPendingRowAfterIdentityAppears(t *testing.T) {
	m, dir := newGroupingManager(t)
	writeChannelFixture(t, dir, "devices", "NEW123 unauthorized\n")
	m.refresh()
	m.identityProbes.Wait()

	devices := m.Devices()
	if len(devices) != 1 || devices[0].Identity != "" || devices[0].IdentitySource != "pending" || devices[0].State != "unauthorized" {
		t.Fatalf("pending row %+v", devices)
	}

	writeChannelFixture(t, dir, "id-NEW123", "Phone123\n")
	writeChannelFixture(t, dir, "id-192.0.2.2:5555", "Phone123\n")
	writeChannelFixture(t, dir, "devices", "NEW123 device transport_id:1\n192.0.2.2:5555 device transport_id:2\n")
	m.refresh()
	m.identityProbes.Wait()

	devices = m.Devices()
	if len(devices) != 1 || devices[0].Identity != "hw:Phone123" || len(devices[0].Connections) != 2 {
		t.Fatalf("pending row did not merge: %+v", devices)
	}
}

func TestAggregateState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		conns []DeviceConnection
		want  string
	}{
		{"online wins", []DeviceConnection{{State: "offline"}, {State: "device"}}, "device"},
		{"all connecting", []DeviceConnection{{State: "connecting"}, {State: "connecting"}}, "connecting"},
		{"unauthorized", []DeviceConnection{{State: "offline"}, {State: "unauthorized"}}, "unauthorized"},
		{"offline", []DeviceConnection{{State: "offline"}}, "offline"},
		{"empty", nil, "offline"},
	} {
		if got := aggregateState(tc.conns); got != tc.want {
			t.Errorf("%s: state=%q want %q", tc.name, got, tc.want)
		}
	}
}
