package logid

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// Device is a handle to a single logid-managed device. The object path and
// name are cached at discovery time; every method issues a fresh D-Bus
// call. If logid restarts and the object is gone, methods return a wrapped
// error and the caller should re-invoke Client.FindDevice.
type Device struct {
	client *Client
	path   dbus.ObjectPath
	name   string
}

// Path returns the device's D-Bus object path.
func (d *Device) Path() dbus.ObjectPath { return d.path }

// Name returns the device name as reported by logid at discovery time.
func (d *Device) Name() string { return d.name }

// FindDevice walks /pizza/pixl/logiops/devices/* and returns the first
// device whose Name property equals name. Returns ErrUnavailable if logid
// is not currently running.
func (c *Client) FindDevice(name string) (*Device, error) {
	if !c.available.Load() {
		return nil, ErrUnavailable
	}

	node, err := introspect.Call(c.conn.Object(busName, devicesPath))
	if err != nil {
		return nil, fmt.Errorf("logid: introspect devices: %w", err)
	}

	for _, path := range parseDeviceChildren(devicesPath, node) {
		obj := c.conn.Object(busName, path)
		v, err := obj.GetProperty(deviceIface + ".Name")
		if err != nil {
			continue
		}
		got, ok := v.Value().(string)
		if !ok {
			continue
		}
		if got == name {
			return &Device{client: c, path: path, name: got}, nil
		}
	}
	return nil, fmt.Errorf("logid: no device named %q", name)
}

// ActiveProfile returns the currently active profile name for the device.
func (d *Device) ActiveProfile() (string, error) {
	if !d.client.available.Load() {
		return "", ErrUnavailable
	}
	obj := d.client.conn.Object(busName, d.path)
	v, err := obj.GetProperty(deviceIface + ".ActiveProfile")
	if err != nil {
		return "", d.wrapErr("get ActiveProfile", err)
	}
	s, ok := v.Value().(string)
	if !ok {
		return "", fmt.Errorf("logid device %q: ActiveProfile: unexpected type %T", d.name, v.Value())
	}
	return s, nil
}

// GetProfiles returns the list of profile names configured for the device.
func (d *Device) GetProfiles() ([]string, error) {
	if !d.client.available.Load() {
		return nil, ErrUnavailable
	}
	obj := d.client.conn.Object(busName, d.path)
	var out []string
	if err := obj.Call(deviceIface+".GetProfiles", 0).Store(&out); err != nil {
		return nil, d.wrapErr("GetProfiles", err)
	}
	return out, nil
}

// SetProfile switches the device's active profile.
func (d *Device) SetProfile(name string) error {
	if !d.client.available.Load() {
		return ErrUnavailable
	}
	obj := d.client.conn.Object(busName, d.path)
	if err := obj.Call(deviceIface+".SetProfile", 0, name).Store(); err != nil {
		return d.wrapErr(fmt.Sprintf("SetProfile(%q)", name), err)
	}
	return nil
}

func (d *Device) wrapErr(op string, err error) error {
	if errors.Is(err, ErrUnavailable) {
		return err
	}
	// If logid restarted and the object we cached is gone, flip availability
	// off so subsequent calls short-circuit to ErrUnavailable until the
	// presence loop sees logid come back up.
	if isNoObjectError(err) {
		d.client.available.Store(false)
	}
	return fmt.Errorf("logid device %q: %s: %w", d.name, op, err)
}
