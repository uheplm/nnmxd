// Package device locates the Logitech MX Master 3 / 3S evdev device across
// both the Unifying/Bolt USB receiver and Bluetooth transports.
package device

import (
	"errors"
	"fmt"
	"strings"

	"github.com/holoplot/go-evdev"

	"github.com/naudachu/mxmoused/internal/config"
)

// LogitechVendorID is the USB vendor id Logitech uses across both wired
// receivers and BLE firmware.
const LogitechVendorID = 0x046d

// Open returns an opened evdev device according to cfg. If cfg.Path is set it
// is used verbatim; otherwise Open scans all input devices and picks the one
// that matches the Logitech vendor id and advertises triggerButton as a key.
func Open(cfg config.DeviceConfig) (*evdev.InputDevice, error) {
	if cfg.Path != "" {
		return evdev.Open(cfg.Path)
	}
	if !cfg.AutoDetect {
		return nil, errors.New("device.path is empty and device.auto_detect is false")
	}
	return findByTrigger(cfg.TriggerButton)
}

func findByTrigger(triggerButton string) (*evdev.InputDevice, error) {
	triggerCode, ok := evdev.KEYFromString[triggerButton]
	if !ok {
		return nil, fmt.Errorf("unknown trigger button %q", triggerButton)
	}
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, fmt.Errorf("list input devices: %w", err)
	}
	for _, p := range paths {
		dev, err := evdev.Open(p.Path)
		if err != nil {
			continue
		}
		if matches(dev, triggerCode) {
			return dev, nil
		}
		dev.Close()
	}
	return nil, fmt.Errorf("no Logitech device advertising %s found", triggerButton)
}

func matches(dev *evdev.InputDevice, triggerCode evdev.EvCode) bool {
	id, err := dev.InputID()
	if err != nil || id.Vendor != LogitechVendorID {
		return false
	}
	name, _ := dev.Name()
	lname := strings.ToLower(name)
	// Covers wired receiver ("Logitech USB Receiver Mouse") and BLE
	// ("MX Master 3" / "MX Master 3S"). The BTN_FORWARD capability check
	// excludes the keyboard-shaped sub-device exposed by the same receiver.
	if !strings.Contains(lname, "mx master") &&
		!strings.Contains(lname, "receiver") &&
		!strings.Contains(lname, "mouse") {
		return false
	}
	for _, c := range dev.CapableEvents(evdev.EV_KEY) {
		if c == triggerCode {
			return true
		}
	}
	return false
}
