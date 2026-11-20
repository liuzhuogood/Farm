package device

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"
	"unicode"

	"farm/internal/dependencies"
)

const (
	identityPending   = "pending"
	maxIdentityProbes = 4
	identityTimeout   = 5 * time.Second
)

// DeviceConnection describes one ADB transport. LastSeenAt uses Unix milliseconds.
type DeviceConnection struct {
	Kind       string `json:"kind"`
	Serial     string `json:"serial"`
	Address    string `json:"address"`
	State      string `json:"state"`
	Online     bool   `json:"online"`
	Active     bool   `json:"active"`
	LastSeenAt int64  `json:"last_seen_at"`
}

type deviceChannel struct {
	connection   DeviceConnection
	transportID  string
	identity     string
	source       string
	lastIdentity string
	stale        bool           // identity must be resampled on the next online epoch
	probe        *identityProbe // one attempt per online epoch, including failed attempts
}

type identityProbe struct {
	cancel context.CancelFunc
}

// refreshChannels uses only ADB observations to decide whether shell commands
// are allowed. A manual record may still carry an older database state.
func (m *Manager) refreshChannels(devices []Device, adbDevices map[string]Device, reconnectedSerial string, now int64) {
	for serial, ch := range m.channels {
		adbDev := adbDevices[serial]
		if adbDev.State != "device" || adbDev.TransportID != ch.transportID || serial == reconnectedSerial {
			ch.invalidateIdentity()
		}
		if adbDev.Serial == "" {
			ch.connection.State = "offline"
			ch.connection.Online = false
		}
	}

	for _, d := range devices {
		adbDev, observed := adbDevices[d.Serial]
		ch := m.channels[d.Serial]
		if ch == nil {
			ch = &deviceChannel{}
			m.channels[d.Serial] = ch
		}
		conn := channelConnection(d, adbDev)
		conn.LastSeenAt = ch.connection.LastSeenAt
		if observed {
			conn.LastSeenAt = now
		}
		ch.connection = conn
		ch.transportID = adbDev.TransportID
		if conn.Online && ch.probe == nil && (ch.identity == "" || ch.stale) &&
			m.probesRunning < maxIdentityProbes && m.identityCtx.Err() == nil {
			ctx, cancel := context.WithTimeout(m.identityCtx, identityTimeout)
			probe := &identityProbe{cancel: cancel}
			ch.probe = probe
			m.probesRunning++
			m.identityProbes.Add(1)
			go m.probeChannelIdentity(ctx, d.Serial, probe)
		}
	}
}

func channelConnection(d, adbDev Device) DeviceConnection {
	conn := DeviceConnection{Serial: d.Serial, Kind: "usb", State: "offline"}
	serial := strings.TrimSuffix(d.Serial, ".")
	_, _, hostPortErr := net.SplitHostPort(serial)
	mdns := strings.HasSuffix(serial, "._adb-tls-connect._tcp") ||
		(strings.HasPrefix(serial, "adb-") && strings.Contains(strings.TrimPrefix(serial, "adb-"), "-"))
	switch {
	case adbDev.adbUSB:
	case mdns:
		conn.Kind = "wifi"
	case hostPortErr == nil || net.ParseIP(serial) != nil || d.Source == "manual":
		conn.Kind, conn.Address = "wifi", d.Serial
	}
	if adbDev.Serial != "" {
		conn.State = adbDev.State
		conn.Online = adbDev.State == "device"
	} else if d.State == "connecting" {
		conn.State = "connecting"
	}
	return conn
}

// invalidateIdentity keeps the last identity for grouping (an unplugged USB
// channel must stay on the same device card) and marks it for resampling on the
// next online epoch, which covers transport changes and address reuse.
func (ch *deviceChannel) invalidateIdentity() {
	if ch.probe != nil {
		ch.probe.cancel()
		ch.probe = nil
	}
	ch.stale = true
}

// ADB returns the canonical target in its successful connect messages.
func connectedADBSerial(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"connected to ", "already connected to "} {
			if serial, ok := strings.CutPrefix(line, prefix); ok {
				return serial
			}
		}
	}
	return ""
}

func (m *Manager) probeChannelIdentity(ctx context.Context, serial string, probe *identityProbe) {
	defer m.identityProbes.Done()
	defer probe.cancel()
	identity, source := m.readChannelIdentity(ctx, serial)

	m.mu.Lock()
	m.probesRunning--
	ch := m.channels[serial]
	if m.identityCtx.Err() != nil || ch == nil || ch.probe != probe {
		m.mu.Unlock()
		return
	}
	if identity != "" {
		ch.identity, ch.source = identity, source
		ch.stale = false
	}
	old := ch.lastIdentity
	if identity != "" {
		ch.lastIdentity = identity
	}
	m.mu.Unlock()

	switch {
	case identity == "":
		log.Printf("[Device] channel identity unavailable serial=%s timeout=%t", serial, ctx.Err() == context.DeadlineExceeded)
	case old != "" && old != identity:
		log.Printf("[Device] channel identity changed serial=%s old=%s new=%s", serial, old, identity)
	}
}

func (m *Manager) readChannelIdentity(ctx context.Context, serial string) (identity, source string) {
	for _, candidate := range []struct {
		source, prefix string
		args           []string
	}{
		{"ro.serialno", "hw:", []string{"getprop", "ro.serialno"}},
		{"ro.boot.serialno", "hw:", []string{"getprop", "ro.boot.serialno"}},
		{"android_id", "aid:", []string{"settings", "get", "secure", "android_id"}},
	} {
		if ctx.Err() != nil {
			break
		}
		args := append([]string{"-s", serial, "shell"}, candidate.args...)
		cmd := dependencies.CommandContext(dependencies.BackgroundADB(ctx), m.adbPath, args...)
		cmd.Dir = os.TempDir()
		out, err := cmd.Output()
		if err == nil {
			if value := validIdentityValue(string(out)); value != "" {
				return candidate.prefix + value, candidate.source
			}
		}
	}
	return "", identityPending
}

func validIdentityValue(output string) string {
	value := strings.TrimSpace(output)
	if len(value) < 4 || len(value) > 64 || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return ""
	}
	switch strings.ToLower(value) {
	case "unknown", "null", "0123456789abcdef":
		return ""
	}
	if strings.Trim(value, "0") == "" {
		return ""
	}
	return value
}
