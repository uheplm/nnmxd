// Package config loads, validates, and provides defaults for mxmoused's
// YAML configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeGesture    Mode = "gesture"
	ModeMetaToggle Mode = "meta-toggle"
)

type Config struct {
	Mode       Mode             `yaml:"mode"`
	Device     DeviceConfig     `yaml:"device"`
	Gesture    GestureConfig    `yaml:"gesture"`
	MetaToggle MetaToggleConfig `yaml:"meta_toggle"`
	Logid      LogidConfig      `yaml:"logid"`
}

type DeviceConfig struct {
	AutoDetect    bool   `yaml:"auto_detect"`
	Path          string `yaml:"path"`
	TriggerButton string `yaml:"trigger_button"`
}

type GestureConfig struct {
	Threshold  int             `yaml:"threshold"`
	Escalation int             `yaml:"escalation"`
	Bindings   GestureBindings `yaml:"bindings"`
}

type GestureBindings struct {
	Left       string `yaml:"left"`
	Right      string `yaml:"right"`
	Up         string `yaml:"up"`
	Down       string `yaml:"down"`
	Tap        string `yaml:"tap"`
	ScrollUp   string `yaml:"scroll_up"`
	ScrollDown string `yaml:"scroll_down"`
}

type MetaToggleConfig struct {
	Key       string `yaml:"key"`
	StateFile string `yaml:"state_file"`
	Waybar    bool   `yaml:"waybar"`
}

type LogidConfig struct {
	// DeviceName is the device name as it appears in logid.cfg. When empty,
	// mxmoused tries "MX Master 3S" and then "MX Master 3" in order.
	DeviceName string `yaml:"device_name"`
}

// Defaults returns a Config with sane built-in defaults. YAML unmarshal onto a
// pre-populated Defaults() preserves defaults for keys omitted from the file.
func Defaults() Config {
	return Config{
		Mode: ModeGesture,
		Device: DeviceConfig{
			AutoDetect:    true,
			TriggerButton: "BTN_FORWARD",
		},
		Gesture: GestureConfig{
			Threshold:  100,
			Escalation: 250,
			Bindings: GestureBindings{
				Left:       "focus-column-left",
				Right:      "focus-column-right",
				Up:         "focus-workspace-up",
				Down:       "focus-workspace-down",
				Tap:        "toggle-overview",
				ScrollUp:   "focus-workspace-up",
				ScrollDown: "focus-workspace-down",
			},
		},
		MetaToggle: MetaToggleConfig{
			Key:       "KEY_LEFTMETA",
			StateFile: "/tmp/mxmoused-state",
			Waybar:    true,
		},
	}
}

// SearchPaths returns the config locations tried when --config is not given,
// in the order they are tried.
func SearchPaths() []string {
	var paths []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "mxmoused", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "mxmoused", "config.yaml"))
	}
	paths = append(paths, "/etc/mxmoused/config.yaml")
	return paths
}

// Load reads YAML from path, or from SearchPaths() if path is empty. Missing
// keys fall back to Defaults(). The returned Config is validated.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, resolved, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", resolved, err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func readConfig(path string) ([]byte, string, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, path, fmt.Errorf("read %s: %w", path, err)
		}
		return data, path, nil
	}
	for _, p := range SearchPaths() {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, p, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, p, fmt.Errorf("read %s: %w", p, err)
		}
	}
	// No config file found — Defaults() alone will be used.
	return nil, "", nil
}

func (c *Config) Validate() error {
	switch c.Mode {
	case ModeGesture, ModeMetaToggle:
	default:
		return fmt.Errorf("unknown mode %q (want %q or %q)",
			c.Mode, ModeGesture, ModeMetaToggle)
	}
	if c.Device.TriggerButton == "" {
		return errors.New("device.trigger_button is required")
	}
	if !c.Device.AutoDetect && c.Device.Path == "" {
		return errors.New("device.path is required when device.auto_detect is false")
	}
	if c.Mode == ModeGesture {
		if c.Gesture.Threshold <= 0 {
			return errors.New("gesture.threshold must be > 0")
		}
		if c.Gesture.Escalation < 0 {
			return errors.New("gesture.escalation must be >= 0")
		}
	}
	if c.Mode == ModeMetaToggle {
		if c.MetaToggle.Key == "" {
			return errors.New("meta_toggle.key is required")
		}
		if c.MetaToggle.Waybar && c.MetaToggle.StateFile == "" {
			return errors.New("meta_toggle.state_file is required when meta_toggle.waybar is true")
		}
	}
	return nil
}
