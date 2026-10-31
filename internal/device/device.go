package device

import (
	"errors"
	"fmt"
	"strings"

	"github.com/holoplot/go-evdev"

	"github.com/uheplm/nnmxd/internal/config"
)

const LogitechVendorID = 0x046d

func Open(settings config.Device) (*evdev.InputDevice, error) {
	if settings.Path != "" {
		return evdev.Open(settings.Path)
	}
	if !settings.AutoDetect {
		return nil, errors.New("device path is unset and auto-detection is disabled")
	}
	trigger, ok := evdev.KEYFromString[settings.TriggerButton]
	if !ok {
		return nil, fmt.Errorf("unknown trigger button %q", settings.TriggerButton)
	}
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, fmt.Errorf("list input devices: %w", err)
	}
	for _, path := range paths {
		dev, err := evdev.Open(path.Path)
		if err != nil {
			continue
		}
		if matches(dev, trigger) {
			return dev, nil
		}
		_ = dev.Close()
	}
	return nil, fmt.Errorf("no Logitech mouse advertising %s was found", settings.TriggerButton)
}

func matches(dev *evdev.InputDevice, trigger evdev.EvCode) bool {
	id, err := dev.InputID()
	if err != nil || id.Vendor != LogitechVendorID {
		return false
	}
	name, _ := dev.Name()
	lowerName := strings.ToLower(name)
	if !strings.Contains(lowerName, "mouse") &&
		!strings.Contains(lowerName, "receiver") &&
		!strings.Contains(lowerName, "mx master") {
		return false
	}
	for _, code := range dev.CapableEvents(evdev.EV_KEY) {
		if code == trigger {
			return true
		}
	}
	return false
}
