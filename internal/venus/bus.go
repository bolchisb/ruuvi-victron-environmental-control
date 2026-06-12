// Package venus is a thin client over the Victron Venus OS D-Bus.
//
// Every value on the Venus D-Bus is an object implementing the
// com.victronenergy.BusItem interface, with GetValue / GetText / SetValue
// methods. We connect to the system bus via godbus (pure Go, no cgo), which
// honours DBUS_SYSTEM_BUS_ADDRESS for off-device development.
package venus

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const busItemIface = "com.victronenergy.BusItem"

// ErrNotConnected is returned by bus operations when there is no live system
// bus (e.g. running off-device). The caller keeps working without telemetry.
var ErrNotConnected = errors.New("d-bus not connected")

// Bus is a connection to the Venus system D-Bus.
type Bus struct {
	conn *dbus.Conn
}

// Connect opens the Venus system bus. On failure it still returns a usable Bus
// whose operations report ErrNotConnected, so the rest of the app can run
// without a bus present.
func Connect() (*Bus, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return &Bus{}, fmt.Errorf("connect system bus: %w", err)
	}
	return &Bus{conn: conn}, nil
}

// Connected reports whether a live system bus is available.
func (b *Bus) Connected() bool { return b.conn != nil }

// Close releases the bus connection.
func (b *Bus) Close() error {
	if b.conn == nil {
		return nil
	}
	return b.conn.Close()
}

// get reads the raw value behind a BusItem.
func (b *Bus) get(service, path string) (interface{}, error) {
	if b.conn == nil {
		return nil, ErrNotConnected
	}
	obj := b.conn.Object(service, dbus.ObjectPath(path))
	var v dbus.Variant
	if err := obj.Call(busItemIface+".GetValue", 0).Store(&v); err != nil {
		return nil, fmt.Errorf("GetValue %s%s: %w", service, path, err)
	}
	return v.Value(), nil
}

// GetFloat reads a numeric BusItem and returns it as float64.
func (b *Bus) GetFloat(service, path string) (float64, error) {
	v, err := b.get(service, path)
	if err != nil {
		return 0, err
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("%s%s: unexpected value type %T", service, path, v)
	}
}

// GetString reads a text BusItem (e.g. a sensor's CustomName).
func (b *Bus) GetString(service, path string) (string, error) {
	v, err := b.get(service, path)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s%s: unexpected value type %T", service, path, v)
	}
	return s, nil
}

// Names lists the well-known service names currently on the bus.
func (b *Bus) Names() ([]string, error) {
	if b.conn == nil {
		return nil, ErrNotConnected
	}
	var names []string
	if err := b.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil, fmt.Errorf("list names: %w", err)
	}
	return names, nil
}

// SetInt writes an integer to a BusItem (e.g. a relay state).
func (b *Bus) SetInt(service, path string, value int32) error {
	if b.conn == nil {
		return ErrNotConnected
	}
	obj := b.conn.Object(service, dbus.ObjectPath(path))
	call := obj.Call(busItemIface+".SetValue", 0, dbus.MakeVariant(value))
	if call.Err != nil {
		return fmt.Errorf("SetValue %s%s=%d: %w", service, path, value, call.Err)
	}
	return nil
}
