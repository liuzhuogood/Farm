package device

import (
	"bytes"
	"context"
	"farm/internal/dependencies"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"farm/internal/db"
)

// Device 表示一台设备，融合了 adb 动态状态与持久化配置。
type Device struct {
	Serial             string   `json:"serial"`
	State              string   `json:"state"`  // "device", "offline", "connecting", "unauthorized" 等
	Source             string   `json:"source"` // "auto" (自动发现) 或 "manual" (手动添加)
	Alias              string   `json:"alias,omitempty"`
	Identifier         string   `json:"identifier"`
	Product            string   `json:"product,omitempty"`
	Model              string   `json:"model,omitempty"`
	DeviceName         string   `json:"device,omitempty"`
	TransportID        string   `json:"transport_id,omitempty"`
	AutoReconnect      bool     `json:"auto_reconnect"`
	Brand              string   `json:"brand,omitempty"`
	Manufacturer       string   `json:"manufacturer,omitempty"`
	AndroidRelease     string   `json:"android_release,omitempty"`
	SDKVersion         int      `json:"sdk_version,omitempty"`
	CPUABI             string   `json:"cpu_abi,omitempty"`
	BuildDisplayID     string   `json:"build_display_id,omitempty"`
	BatteryLevel       *int     `json:"battery_level,omitempty"`
	BatteryTemp        *float64 `json:"battery_temp,omitempty"`
	BatteryCharging    *bool    `json:"battery_charging,omitempty"`
	CPUTemp            *float64 `json:"cpu_temp,omitempty"`
	CPUTempSupported   bool     `json:"cpu_temp_supported"`
	ForegroundPkg      string   `json:"foreground_pkg,omitempty"`
	ForegroundAppName  string   `json:"foreground_app_name,omitempty"`
	ForegroundAppIcon  string   `json:"foreground_app_icon,omitempty"`
	ScreenOn           *bool    `json:"screen_on,omitempty"`
	ScreenLocked       *bool    `json:"screen_locked,omitempty"`
	TelemetryUpdatedAt int64    `json:"telemetry_updated_at,omitempty"`

	Identity       string             `json:"identity"`
	IdentitySource string             `json:"identity_source"`
	ActiveKind     string             `json:"active_kind"`
	Connections    []DeviceConnection `json:"connections"`
	adbUSB         bool
}

// IdentifierName derives the editable public device identifier from the saved
// alias, then the reported brand, then the exact serial. Models are not aliases.
func (d Device) IdentifierName() string {
	if alias := strings.TrimSpace(d.Alias); alias != "" {
		return alias
	}
	if brand := strings.TrimSpace(d.Brand); brand != "" {
		return brand
	}
	return d.Serial
}

// Manager 负责跟踪 adb 设备列表并守护手动添加设备的连接与持久化。
type Manager struct {
	adbPath         string
	db              *db.DB
	mu              sync.RWMutex
	aliasMu         sync.Mutex
	aliasRevision   uint64
	devices         []Device
	stop            chan struct{}
	telemetryMu     sync.RWMutex
	hardwareCache   map[string]HardwareInfo
	telemetryCache  map[string]TelemetryInfo
	fetchingHW      map[string]bool
	refreshMu       sync.Mutex
	channels        map[string]*deviceChannel // guarded by mu
	identityAliases map[string]string         // device identity → alias, guarded by mu
	identityCtx     context.Context
	cancelIdentity  context.CancelFunc
	identityProbes  sync.WaitGroup
	probesRunning   int // guarded by mu

	// 自动恢复状态：网络设备重连计数、日志节流与 ADB 服务重启冷却。
	reconnectMu     sync.Mutex
	reconnectFails  map[string]int
	reconnecting    map[string]bool
	lastLogAt       map[string]time.Time
	restartInFlight bool
	lastRestartAt   time.Time
	devicesFails    int
}

const (
	// 连续失败达到该次数后先断开旧传输，再重新 connect。
	reconnectDisconnectAfter = 1
	// 无任何在线设备且重连连续失败达到该次数时，自动重启 ADB 服务。
	reconnectRestartAfter = 3
	// adb devices 连续失败达到该次数（约 10 秒）后重启 ADB 服务。
	devicesRestartAfter   = 5
	restartADBMinInterval = 2 * time.Minute
	reconnectLogInterval  = 30 * time.Second
)

// NewManager 创建设备管理器。
func NewManager(adbPath string, database *db.DB) *Manager {
	identityCtx, cancelIdentity := context.WithCancel(context.Background())
	return &Manager{
		adbPath:         adbPath,
		db:              database,
		stop:            make(chan struct{}),
		hardwareCache:   make(map[string]HardwareInfo),
		telemetryCache:  make(map[string]TelemetryInfo),
		fetchingHW:      make(map[string]bool),
		channels:        make(map[string]*deviceChannel),
		identityAliases: make(map[string]string),
		identityCtx:     identityCtx,
		cancelIdentity:  cancelIdentity,
		reconnectFails:  make(map[string]int),
		reconnecting:    make(map[string]bool),
		lastLogAt:       make(map[string]time.Time),
	}
}

// Start 启动后台轮询，定期刷新设备列表和自动重连守护。
func (m *Manager) Start() {
	go m.loop()
	go m.telemetryLoop()
}

// Stop 停止轮询。
func (m *Manager) Stop() {
	close(m.stop)
	m.cancelIdentity()
	// Synchronize with the last refresh that may have started a probe.
	m.mu.Lock()
	m.mu.Unlock()
	m.identityProbes.Wait()
}

func (m *Manager) loop() {
	m.refresh()
	ticker := time.NewTicker(2 * time.Second)
	reconnectTicker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	defer reconnectTicker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.refresh()
		case <-reconnectTicker.C:
			m.reconnectOfflineManualDevices()
		}
	}
}

// RestartADB 重启本机 ADB 服务：先 kill-server 再 start-server。
// kill-server 失败（如服务本未运行）只记录日志，仍继续启动。
func (m *Manager) RestartADB(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if out, err := dependencies.CommandContext(ctx, m.adbPath, "kill-server").CombinedOutput(); err != nil {
		log.Printf("[Device] adb kill-server 未成功，继续启动服务: %v: %s", err, strings.TrimSpace(string(out)))
	}
	out, err := dependencies.CommandContext(ctx, m.adbPath, "start-server").CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb start-server 失败: %w: %s", err, strings.TrimSpace(string(out)))
	}
	log.Printf("[Device] ADB 服务已重启")
	return nil
}

// refresh 执行 adb devices -l 并与 SQLite 手动设备合并。
func (m *Manager) refresh() {
	m.refreshWithReconnect("")
}

func (m *Manager) refreshWithReconnect(reconnectedSerial string) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.RLock()
	aliasRevision := m.aliasRevision
	m.mu.RUnlock()
	adbDevs, err := m.ListDevices()
	if err != nil {
		// 读不到列表时保留上一份设备快照，避免把在线设备误标为离线；
		// 连续失败（服务卡死/无法连接守护进程）则在冷却窗口内自动重启。
		n := m.recordDevicesFailure()
		m.logThrottled("adb-devices", "[Device] adb devices 失败（连续 %d 次）: %v", n, err)
		if n >= devicesRestartAfter {
			m.escalateRestartADB("adb devices 连续失败: " + err.Error())
		}
		return
	}
	m.clearDevicesFailure()

	adbMap := make(map[string]Device)
	for _, d := range adbDevs {
		adbMap[d.Serial] = d
	}

	var manualList []db.DeviceRecord
	var aliases map[string]string
	if m.db != nil {
		manualList, _ = m.db.ListManualDevices()
		var aliasErr error
		aliases, aliasErr = m.db.ListDeviceAliases()
		if aliasErr != nil {
			log.Printf("[Device] load identifiers failed, retaining current snapshot: %v", aliasErr)
			return
		}
	}

	manualMap := make(map[string]db.DeviceRecord)
	for _, rec := range manualList {
		manualMap[rec.Serial] = rec
	}

	var combined []Device
	seen := make(map[string]bool)

	// 1. 处理数据库中的手动设备
	for _, rec := range manualList {
		seen[rec.Serial] = true
		initialState := rec.State
		if initialState == "" {
			initialState = "offline"
		}
		d := Device{
			Serial:        rec.Serial,
			Source:        "manual",
			Alias:         rec.Alias,
			State:         initialState,
			Product:       rec.Product,
			Model:         rec.Model,
			DeviceName:    rec.DeviceName,
			TransportID:   rec.TransportID,
			AutoReconnect: rec.AutoReconnect == 1,
		}

		if adbDev, ok := adbMap[rec.Serial]; ok {
			d.State = adbDev.State
			if adbDev.Product != "" {
				d.Product = adbDev.Product
			}
			if adbDev.Model != "" {
				d.Model = adbDev.Model
			}
			if adbDev.DeviceName != "" {
				d.DeviceName = adbDev.DeviceName
			}
			if adbDev.TransportID != "" {
				d.TransportID = adbDev.TransportID
			}

			// 异步回写 DB 最新状态
			if m.db != nil && (rec.State != d.State || rec.Model != d.Model) {
				go m.db.UpdateDeviceStatus(d.Serial, d.State, d.Product, d.Model, d.DeviceName, d.TransportID, d.State == "device")
			}
		} else {
			if m.db != nil && rec.State != "offline" && rec.State != "connecting" {
				go m.db.UpdateDeviceStatus(d.Serial, "offline", "", "", "", "", false)
			}
		}
		combined = append(combined, d)
	}

	// 2. 处理 ADB 自动发现的设备（未在手动列表中的）
	for _, adbDev := range adbDevs {
		if !seen[adbDev.Serial] {
			adbDev.Source = "auto"
			adbDev.Alias = aliases[adbDev.Serial]
			combined = append(combined, adbDev)
		}
	}

	m.mu.Lock()
	// A channel that dropped off `adb devices` but belongs to an identity with
	// another live channel stays on the card as an offline connection.
	combined = m.retainMergedChannelsLocked(combined)
	// A refresh may have read the database before a rename completed. Preserve
	// the current aliases when publishing that older device snapshot.
	if m.aliasRevision != aliasRevision {
		latestAliases := make(map[string]string, len(m.devices))
		for _, d := range m.devices {
			latestAliases[d.Serial] = d.Alias
		}
		for i := range combined {
			if alias, ok := latestAliases[combined[i].Serial]; ok {
				combined[i].Alias = alias
			}
		}
	}
	m.refreshChannels(combined, adbMap, reconnectedSerial, time.Now().UnixMilli())
	m.devices = combined
	m.mu.Unlock()
}

// retainMergedChannelsLocked appends offline rows for channels that vanished
// from `adb devices` while another channel of the same identity is still
// present. Without this a dual-link card would lose its USB icon the moment
// the cable is unplugged. Caller must hold m.mu.
func (m *Manager) retainMergedChannelsLocked(combined []Device) []Device {
	present := make(map[string]bool, len(combined))
	identities := make(map[string]bool, len(combined))
	for _, d := range combined {
		present[d.Serial] = true
		if ch := m.channels[d.Serial]; ch != nil && ch.identity != "" {
			identities[ch.identity] = true
		}
	}
	for serial, ch := range m.channels {
		if present[serial] || ch.identity == "" || !identities[ch.identity] {
			continue
		}
		combined = append(combined, Device{Serial: serial, Source: "auto", State: "offline"})
	}
	return combined
}

// attachTelemetry 将缓存的静态硬件信息与最新遥测指标附加至 Device 实例。
// 设备级数据按身份缓存，同一台设备切换通道后仍然命中。
func (m *Manager) attachTelemetry(d *Device) {
	key := m.telemetryKey(d)
	m.telemetryMu.RLock()
	hw, hasHW := m.hardwareCache[key]
	tel, hasTel := m.telemetryCache[key]
	isFetching := m.fetchingHW[key]
	m.telemetryMu.RUnlock()

	if hasHW {
		if hw.Brand != "" {
			d.Brand = hw.Brand
		}
		if hw.Manufacturer != "" {
			d.Manufacturer = hw.Manufacturer
		}
		if hw.Model != "" && d.Model == "" {
			d.Model = hw.Model
		}
		d.AndroidRelease = hw.AndroidRelease
		d.SDKVersion = hw.SDKVersion
		d.CPUABI = hw.CPUABI
		d.BuildDisplayID = hw.BuildDisplayID
	} else if d.State == "device" && !isFetching {
		m.telemetryMu.Lock()
		m.fetchingHW[key] = true
		m.telemetryMu.Unlock()
		go m.runHardwareFetch(key, d.Serial)
	}

	if hasTel {
		d.BatteryLevel = tel.BatteryLevel
		d.BatteryTemp = tel.BatteryTemp
		d.BatteryCharging = tel.BatteryCharging
		d.CPUTemp = tel.CPUTemp
		d.CPUTempSupported = tel.CPUTempSupported
		d.ForegroundPkg = tel.ForegroundPkg
		d.ForegroundAppName = tel.ForegroundAppName
		d.ForegroundAppIcon = tel.ForegroundAppIcon
		d.ScreenOn = tel.ScreenOn
		d.ScreenLocked = tel.ScreenLocked
		d.TelemetryUpdatedAt = tel.UpdatedAt
	}
}

// reconnectOfflineManualDevices 扫描开启了 auto_reconnect 且不在线的网络设备，
// 按连续失败次数升级恢复手段：重连 → 断开旧传输后重连 → 兜底重启 ADB 服务。
func (m *Manager) reconnectOfflineManualDevices() {
	if m.db == nil {
		return
	}
	manuals, err := m.db.ListManualDevices()
	if err != nil {
		m.logThrottled("manual-list", "[Device] 读取手动设备失败: %v", err)
		return
	}

	m.mu.RLock()
	states := make(map[string]string, len(m.devices))
	online := 0
	for _, d := range m.devices {
		states[d.Serial] = d.State
		if d.State == "device" {
			online++
		}
	}
	m.mu.RUnlock()

	var broken []string
	for _, rec := range manuals {
		if rec.AutoReconnect != 1 {
			continue
		}
		state, known := states[rec.Serial]
		if !known {
			state = rec.State
		}
		if state == "device" {
			m.clearReconnectFailure(rec.Serial)
			continue
		}
		// 只有网络设备能通过 disconnect/connect 重建传输；USB 通道交给 adb 自身。
		if strings.Contains(rec.Serial, ":") {
			broken = append(broken, rec.Serial)
		}
	}

	maxFailures := 0
	for _, serial := range broken {
		if n := m.reconnectFailureCount(serial); n > maxFailures {
			maxFailures = n
		}
		if m.beginReconnect(serial) {
			go func(serial string) {
				defer m.endReconnect(serial)
				m.reconnectNetworkDevice(serial)
			}(serial)
		}
	}

	// 全部设备都不可用时才自动重启 ADB，避免打断仍在线的投屏会话。
	if len(broken) > 0 && online == 0 && maxFailures >= reconnectRestartAfter {
		m.escalateRestartADB(fmt.Sprintf("全部设备离线且重连连续失败 %d 次（%s）", maxFailures, strings.Join(broken, ",")))
	}
}

func (m *Manager) reconnectNetworkDevice(serial string) {
	// 传输已僵死时 adb connect 只回 "already connected"，不会重建连接；
	// 第二次尝试起先移除旧条目。
	if m.reconnectFailureCount(serial) >= reconnectDisconnectAfter {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := dependencies.CommandContext(ctx, m.adbPath, "disconnect", serial).CombinedOutput()
		cancel()
		if err != nil {
			// 传输已经不存在时 adb 返回 no such device，属于预期结果，不算故障。
			if msg := strings.TrimSpace(string(out)); !strings.Contains(msg, "no such device") {
				m.logThrottled("disconnect:"+serial, "[Device] adb disconnect 失败 serial=%s: %v: %s", serial, err, msg)
			}
		}
	}

	// Connect 内部已记录失败原因；这里只根据刷新后的真实状态判断是否恢复。
	_ = m.Connect(serial)
	if m.deviceState(serial) == "device" {
		m.clearReconnectFailure(serial)
		return
	}
	failures := m.recordReconnectFailure(serial)
	m.logThrottled("reconnect:"+serial, "[Device] 自动重连未恢复 serial=%s 连续失败=%d", serial, failures)
}

func (m *Manager) deviceState(serial string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.devices {
		if m.devices[i].Serial == serial {
			return m.devices[i].State
		}
	}
	return ""
}

func (m *Manager) recordDevicesFailure() int {
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()
	m.devicesFails++
	return m.devicesFails
}

func (m *Manager) clearDevicesFailure() {
	m.reconnectMu.Lock()
	m.devicesFails = 0
	m.reconnectMu.Unlock()
}

func (m *Manager) reconnectFailureCount(serial string) int {
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()
	return m.reconnectFails[serial]
}

func (m *Manager) recordReconnectFailure(serial string) int {
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()
	m.reconnectFails[serial]++
	return m.reconnectFails[serial]
}

func (m *Manager) clearReconnectFailure(serial string) {
	m.reconnectMu.Lock()
	delete(m.reconnectFails, serial)
	m.reconnectMu.Unlock()
}

func (m *Manager) beginReconnect(serial string) bool {
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()
	if m.reconnecting[serial] {
		return false
	}
	m.reconnecting[serial] = true
	return true
}

func (m *Manager) endReconnect(serial string) {
	m.reconnectMu.Lock()
	delete(m.reconnecting, serial)
	m.reconnectMu.Unlock()
}

// escalateRestartADB 在冷却窗口内只执行一次重启，避免故障时反复打断 ADB 服务。
func (m *Manager) escalateRestartADB(reason string) {
	now := time.Now()
	m.reconnectMu.Lock()
	if m.restartInFlight || now.Sub(m.lastRestartAt) < restartADBMinInterval {
		m.reconnectMu.Unlock()
		return
	}
	m.restartInFlight = true
	m.lastRestartAt = now
	m.reconnectMu.Unlock()

	go func() {
		defer func() {
			m.reconnectMu.Lock()
			m.restartInFlight = false
			m.reconnectMu.Unlock()
		}()
		log.Printf("[Device] 自动重启 ADB 服务: %s", reason)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := m.RestartADB(ctx); err != nil {
			log.Printf("[Device] 自动重启 ADB 服务失败: %v", err)
			return
		}
		m.refresh()
	}()
}

// logThrottled 按 key 限制同一故障的日志频率，避免故障期间刷屏。
func (m *Manager) logThrottled(key, format string, args ...interface{}) {
	now := time.Now()
	m.reconnectMu.Lock()
	if last, ok := m.lastLogAt[key]; ok && now.Sub(last) < reconnectLogInterval {
		m.reconnectMu.Unlock()
		return
	}
	m.lastLogAt[key] = now
	m.reconnectMu.Unlock()
	log.Printf(format, args...)
}

// Connect 调用 adb connect <serial>
func (m *Manager) Connect(serial string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := dependencies.CommandContext(ctx, m.adbPath, "connect", serial)
	cmd.Dir = os.TempDir()
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		m.logThrottled("connect:"+serial, "[Device] adb connect 失败 serial=%s: %v: %s", serial, err, output)
		return fmt.Errorf("adb connect 错误: %s (%w)", output, err)
	}

	// 触发一次即刻刷新（包含 failed to authenticate 变为 unauthorized 的情况）
	m.refreshWithReconnect(connectedADBSerial(output))

	// adb connect 成功通常输出: connected to ... 或 already connected to ...
	// 未授权时输出: failed to authenticate to ...（此时已建立连接并处于 unauthorized 状态）
	if (strings.Contains(output, "failed") && !strings.Contains(output, "failed to authenticate")) || strings.Contains(output, "cannot connect") {
		m.logThrottled("connect:"+serial, "[Device] adb connect 未成功 serial=%s: %s", serial, output)
		return fmt.Errorf("adb connect 失败: %s", output)
	}
	// 传输僵死时也会回 already connected；记录一次便于区分“已连上”和“连不上”。
	if strings.Contains(output, "already connected") {
		m.logThrottled("connect:"+serial, "[Device] adb connect 返回已连接 serial=%s", serial)
	}

	return nil
}

// AddManualDevice 添加手动设备并尝试首次连接
func (m *Manager) AddManualDevice(serial, alias string) (*Device, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return nil, fmt.Errorf("设备地址不能为空")
	}

	if m.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	rec, err := m.db.UpsertManualDevice(serial, alias, 1)
	if err != nil {
		return nil, fmt.Errorf("保存手动设备失败: %w", err)
	}

	// 异步尝试 ADB 连接，不阻塞响应
	go func() {
		_ = m.Connect(serial)
	}()

	dev := &Device{
		Serial:        rec.Serial,
		Source:        "manual",
		Alias:         rec.Alias,
		State:         "connecting",
		AutoReconnect: true,
	}
	dev.Identifier = dev.IdentifierName()

	m.mu.Lock()
	found := false
	for i, existing := range m.devices {
		if existing.Serial == dev.Serial {
			m.devices[i] = *dev
			found = true
			break
		}
	}
	if !found {
		m.devices = append(m.devices, *dev)
	}
	result, _ := m.groupedDeviceLocked(dev.Serial)
	m.mu.Unlock()

	go m.refresh()
	return &result, nil
}

// RemoveManualDevice 移除手动设备
func (m *Manager) RemoveManualDevice(serial string) error {
	if m.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	if err := m.db.DeleteManualDevice(serial); err != nil {
		return err
	}

	m.mu.Lock()
	newDevs := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		if d.Serial != serial {
			newDevs = append(newDevs, d)
		}
	}
	m.devices = newDevs
	m.mu.Unlock()

	// 可选尝试断开 adb disconnect
	go func() {
		cmd := dependencies.Command(m.adbPath, "disconnect", serial)
		cmd.Dir = os.TempDir()
		_ = cmd.Run()
	}()

	go m.refresh()
	return nil
}

// UpdateAlias 更新别名
func (m *Manager) UpdateAlias(serial, alias string) error {
	if m.db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	m.aliasMu.Lock()
	defer m.aliasMu.Unlock()
	m.mu.RLock()
	dev, ok := m.groupedDeviceLocked(serial)
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("设备不存在: %s", serial)
	}
	alias = strings.TrimSpace(alias)
	if err := m.db.UpdateAlias(dev.Serial, alias); err != nil {
		return err
	}
	m.mu.Lock()
	for i := range m.devices {
		if m.devices[i].Serial == dev.Serial {
			m.devices[i].Alias = alias
		}
	}
	if dev.Identity != "" {
		m.identityAliases[dev.Identity] = alias
	}
	m.aliasRevision++
	m.mu.Unlock()
	log.Printf("[Device] identifier updated serial=%s identity=%s", dev.Serial, dev.Identity)
	return nil
}

// ListDevices 执行一次 adb devices -l 并返回解析结果。
func (m *Manager) ListDevices() ([]Device, error) {
	cmd := dependencies.Command(m.adbPath, "devices", "-l")
	cmd.Dir = os.TempDir()
	// 服务懒启动时 adb 会把 daemon 提示写到 stderr；成功路径也要带上它，
	// 但设备列表只取 stdout，避免把 "* daemon started successfully" 解析成设备。
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseDevices(string(out)), nil
}

// parseDevices 解析 adb devices -l 的输出。
func parseDevices(s string) []Device {
	var devices []Device
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{
			Serial: fields[0],
			State:  fields[1],
			Source: "auto",
		}
		for _, f := range fields[2:] {
			kv := strings.SplitN(f, ":", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "usb":
				d.adbUSB = true
			case "product":
				d.Product = kv[1]
			case "model":
				d.Model = kv[1]
			case "device":
				d.DeviceName = kv[1]
			case "transport_id":
				d.TransportID = kv[1]
			}
		}
		devices = append(devices, d)
	}
	return devices
}

// Devices 返回按设备身份归并后的当前设备列表（在线或配置的手动设备）。
func (m *Manager) Devices() []Device {
	m.mu.RLock()
	res := m.groupedSnapshotLocked()
	m.mu.RUnlock()
	for i := range res {
		m.attachTelemetry(&res[i])
		res[i].Identifier = res[i].IdentifierName()
	}
	return res
}

// GetDevice 返回指定 serial（任意通道）或 identity 对应的设备。
func (m *Manager) GetDevice(serial string) (Device, bool) {
	m.mu.RLock()
	dev, ok := m.groupedDeviceLocked(serial)
	m.mu.RUnlock()
	if !ok || dev.State != "device" {
		return Device{}, false
	}
	m.attachTelemetry(&dev)
	dev.Identifier = dev.IdentifierName()
	return dev, true
}
