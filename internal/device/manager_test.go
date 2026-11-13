package device

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"farm/internal/db"
)

func TestParseDevices(t *testing.T) {
	output := `List of devices attached
10.0.0.104:5555        device product:PD2072 model:V2072A device:PD2072 transport_id:1
RFCW10JXYZ             device product:panther model:Pixel_7 device:panther transport_id:2
`
	devs := parseDevices(output)
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	if devs[0].Serial != "10.0.0.104:5555" || devs[0].Model != "V2072A" || devs[0].Source != "auto" {
		t.Errorf("unexpected dev 0: %+v", devs[0])
	}
	if devs[1].Serial != "RFCW10JXYZ" || devs[1].Product != "panther" {
		t.Errorf("unexpected dev 1: %+v", devs[1])
	}
}

func TestManagerManualDevices(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mgr-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mgr := NewManager("false", database) // 使用 false 命令使 adb devices 失败，纯测试手动设备
	defer mgr.Stop()

	// 添加手动设备
	dev, err := mgr.AddManualDevice("192.168.1.188:5555", "客厅电视")
	if err != nil {
		t.Fatalf("AddManualDevice failed: %v", err)
	}
	if dev.Serial != "192.168.1.188:5555" || dev.Source != "manual" || dev.Alias != "客厅电视" {
		t.Errorf("unexpected manual dev: %+v", dev)
	}

	// 验证 Devices() 中包含该手动设备，且初始状态为 connecting
	all := mgr.Devices()
	if len(all) != 1 {
		t.Fatalf("expected 1 device, got %d", len(all))
	}
	if all[0].Serial != "192.168.1.188:5555" || all[0].Source != "manual" || all[0].State != "connecting" {
		t.Errorf("unexpected listed dev: %+v", all[0])
	}

	// 更新别名
	err = mgr.UpdateAlias("192.168.1.188:5555", "卧室电视")
	if err != nil {
		t.Fatalf("UpdateAlias failed: %v", err)
	}
	all = mgr.Devices()
	if all[0].Alias != "卧室电视" {
		t.Errorf("alias expected '卧室电视', got '%s'", all[0].Alias)
	}

	// 删除手动设备
	err = mgr.RemoveManualDevice("192.168.1.188:5555")
	if err != nil {
		t.Fatalf("RemoveManualDevice failed: %v", err)
	}
	all = mgr.Devices()
	if len(all) != 0 {
		t.Errorf("expected 0 devices after remove, got %d", len(all))
	}
}

func TestDeviceIdentifierNameAndSnapshotDoNotUseModel(t *testing.T) {
	for _, tc := range []struct {
		device Device
		want   string
	}{
		{Device{Serial: "s", Alias: "  Desk phone  ", Brand: "Redmi", Model: "Pixel"}, "Desk phone"},
		{Device{Serial: "s", Alias: "  ", Brand: " Redmi "}, "Redmi"},
		{Device{Serial: "s", Brand: " ", Model: "Pixel"}, "s"},
	} {
		if got := tc.device.IdentifierName(); got != tc.want {
			t.Fatalf("identifier=%q want %q", got, tc.want)
		}
	}
	mgr := NewManager("false", nil)
	mgr.devices = []Device{{Serial: "s", Brand: "Redmi"}}
	copy := mgr.Devices()
	if copy[0].Identifier != "Redmi" || mgr.devices[0].Identifier != "" {
		t.Fatalf("snapshot derivation changed storage: %#v %#v", copy, mgr.devices)
	}
	copy[0].Alias = "Changed"
	if mgr.devices[0].Alias != "" {
		t.Fatal("returned snapshot mutates registry")
	}
}

func TestAutoDeviceIdentifierPersistsAndReloadsWithoutChangingSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell executable")
	}
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	adbPath := filepath.Join(dir, "adb")
	if err := os.WriteFile(adbPath, []byte("#!/bin/sh\nprintf 'List of devices attached\\nauto-one device model:Pixel\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(adbPath, database)
	mgr.hardwareCache["auto-one"] = HardwareInfo{Brand: "Redmi"}
	mgr.refresh()
	if err := mgr.UpdateAlias("auto-one", "  Desk phone  "); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Devices()[0]; got.Identifier != "Desk phone" || got.Alias != "Desk phone" || got.Source != "auto" {
		t.Fatalf("rename: %#v", got)
	}
	mgr.refresh()
	if got := mgr.Devices()[0]; got.Identifier != "Desk phone" || got.Source != "auto" {
		t.Fatalf("reload: %#v", got)
	}
	manual, err := database.ListManualDevices()
	if err != nil || len(manual) != 0 {
		t.Fatalf("auto rename became manual: %#v %v", manual, err)
	}
	if err := mgr.UpdateAlias("auto-one", "  "); err != nil {
		t.Fatal(err)
	}
	mgr.refresh()
	if got := mgr.Devices()[0]; got.Identifier != "Redmi" || got.Alias != "" {
		t.Fatalf("clear alias: %#v", got)
	}
	if err := mgr.UpdateAlias("unknown", "Another"); err == nil {
		t.Fatal("unknown device persisted")
	}
	aliases, err := database.ListDeviceAliases()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aliases["unknown"]; ok {
		t.Fatal("unknown alias created in DB")
	}
}

func TestAliasReadFailureRetainsCurrentSnapshot(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewManager("false", database)
	mgr.devices = []Device{{Serial: "phone", State: "device", Source: "auto", Alias: "自定义手机", Brand: "Redmi"}}
	database.Close()
	mgr.refresh()
	devices := mgr.Devices()
	if len(devices) != 1 || devices[0].Identifier != "自定义手机" {
		t.Fatalf("failed DB read changed identifier: %+v", devices)
	}
}
