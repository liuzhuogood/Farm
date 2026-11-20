package device

import (
	"log"
	"strings"
)

const (
	connectionUSB  = "usb"
	connectionWifi = "wifi"
)

// groupedSnapshotLocked collapses one row per ADB channel into one entry per
// device identity. Rows whose identity is not collected yet stay separate.
// Caller must hold m.mu.
func (m *Manager) groupedSnapshotLocked() []Device {
	order := make([]string, 0, len(m.devices))
	groups := make(map[string][]Device, len(m.devices))
	for _, d := range m.devices {
		key := m.groupKeyLocked(d.Serial)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], d)
	}
	out := make([]Device, 0, len(order))
	for _, key := range order {
		out = append(out, m.mergeGroupLocked(key, groups[key]))
	}
	return out
}

// groupKeyLocked groups online channels by the identity they reported. A
// channel without identity is its own group until a probe succeeds.
func (m *Manager) groupKeyLocked(serial string) string {
	if ch := m.channels[serial]; ch != nil && ch.identity != "" {
		return ch.identity
	}
	return "serial:" + serial
}

// groupedDeviceLocked returns the merged entry that currently owns serial.
func (m *Manager) groupedDeviceLocked(serial string) (Device, bool) {
	for _, dev := range m.groupedSnapshotLocked() {
		if dev.Identity != "" && dev.Identity == serial {
			return dev, true
		}
		for _, conn := range dev.Connections {
			if conn.Serial == serial {
				return dev, true
			}
		}
	}
	return Device{}, false
}

func (m *Manager) mergeGroupLocked(key string, rows []Device) Device {
	representatives := make(map[string]DeviceConnection, len(rows))
	identity, source := "", identityPending
	alias := ""
	autoReconnect := false
	manual := false
	for _, row := range rows {
		conn := m.rowConnectionLocked(row)
		if best, ok := representatives[conn.Kind]; !ok {
			representatives[conn.Kind] = conn
		} else if preferConnection(conn, best) {
			representatives[conn.Kind] = conn
			log.Printf("[Device] duplicate %s channel ignored identity=%s serial=%s kept=%s", conn.Kind, key, best.Serial, conn.Serial)
		} else {
			log.Printf("[Device] duplicate %s channel ignored identity=%s serial=%s kept=%s", conn.Kind, key, conn.Serial, best.Serial)
		}
		if alias == "" {
			alias = strings.TrimSpace(row.Alias)
		}
		autoReconnect = autoReconnect || row.AutoReconnect
		manual = manual || row.Source == "manual"
		if ch := m.channels[row.Serial]; ch != nil && ch.identity != "" {
			identity = ch.identity
			if ch.source != "" {
				source = ch.source
			}
		}
	}

	connections := make([]DeviceConnection, 0, len(representatives))
	for _, kind := range []string{connectionUSB, connectionWifi} {
		if conn, ok := representatives[kind]; ok {
			connections = append(connections, conn)
		}
	}
	activeKind := ""
	for _, kind := range []string{connectionUSB, connectionWifi} {
		if conn, ok := representatives[kind]; ok && conn.Online {
			activeKind = kind
			break
		}
	}
	for i := range connections {
		connections[i].Active = activeKind != "" && connections[i].Kind == activeKind
	}

	dev := representativeRowLocked(m, rows, representatives, activeKind)
	dev.Identity = identity
	dev.IdentitySource = identityPending
	if identity != "" {
		dev.IdentitySource = source
	}
	dev.ActiveKind = activeKind
	dev.Connections = connections
	dev.State = aggregateState(connections)
	dev.AutoReconnect = autoReconnect
	if manual {
		dev.Source = "manual"
	}
	if dev.Alias == "" {
		dev.Alias = alias
	}
	if identity != "" {
		if named, ok := m.identityAliases[identity]; ok {
			dev.Alias = named
		}
	}
	return dev
}

func representativeRowLocked(m *Manager, rows []Device, representatives map[string]DeviceConnection, activeKind string) Device {
	preferred := []string{}
	if activeKind != "" {
		preferred = append(preferred, activeKind)
	}
	preferred = append(preferred, connectionUSB, connectionWifi)
	for _, kind := range preferred {
		conn, ok := representatives[kind]
		if !ok {
			continue
		}
		for _, row := range rows {
			if row.Serial == conn.Serial {
				return row
			}
		}
	}
	return rows[0]
}

func (m *Manager) rowConnectionLocked(d Device) DeviceConnection {
	if ch := m.channels[d.Serial]; ch != nil && ch.connection.Serial != "" {
		return ch.connection
	}
	return channelConnection(d, Device{})
}

func preferConnection(candidate, current DeviceConnection) bool {
	if candidate.Online != current.Online {
		return candidate.Online
	}
	return candidate.LastSeenAt > current.LastSeenAt
}

// aggregateState derives the device state from its channels: one authorised
// channel is enough to use the device, a device waiting on every channel is
// still connecting, and an unauthorised channel outranks a plain offline one.
func aggregateState(connections []DeviceConnection) string {
	allConnecting := len(connections) > 0
	unauthorized := false
	for _, conn := range connections {
		switch conn.State {
		case "device":
			return "device"
		case "connecting":
		case "unauthorized":
			unauthorized = true
			allConnecting = false
		default:
			allConnecting = false
		}
	}
	if allConnecting {
		return "connecting"
	}
	if unauthorized {
		return "unauthorized"
	}
	return "offline"
}
