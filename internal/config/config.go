package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const Version = 1

type Mode string

const (
	ModeGesture    Mode = "gesture"
	ModeMetaToggle Mode = "meta-toggle"
)

type Device struct {
	AutoDetect    bool   `json:"autoDetect"`
	Path          string `json:"path"`
	TriggerButton string `json:"triggerButton"`
}

type Bindings struct {
	Left       string `json:"left"`
	Right      string `json:"right"`
	Up         string `json:"up"`
	Down       string `json:"down"`
	Tap        string `json:"tap"`
	ScrollUp   string `json:"scrollUp"`
	ScrollDown string `json:"scrollDown"`
}

type Preset struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Mode       Mode     `json:"mode"`
	Threshold  int      `json:"threshold"`
	Escalation int      `json:"escalation"`
	MetaKey    string   `json:"metaKey"`
	Bindings   Bindings `json:"bindings"`
}

type Config struct {
	Version         int               `json:"version"`
	Device          Device            `json:"device"`
	DefaultPresetID string            `json:"defaultPresetId"`
	Presets         []Preset          `json:"presets"`
	AppPresets      map[string]string `json:"appPresets"`
	MetaStateFile   string            `json:"metaStateFile"`
	WriteWaybar     bool              `json:"writeWaybar"`
}

func Defaults() Config {
	return Config{
		Version: Version,
		Device: Device{
			AutoDetect:    true,
			TriggerButton: "BTN_FORWARD",
		},
		DefaultPresetID: "default",
		Presets: []Preset{{
			ID:         "default",
			Name:       "Default",
			Mode:       ModeGesture,
			Threshold:  100,
			Escalation: 250,
			MetaKey:    "KEY_LEFTMETA",
			Bindings: Bindings{
				Left:       "focus-column-left",
				Right:      "focus-column-right",
				Up:         "focus-workspace-up",
				Down:       "focus-workspace-down",
				Tap:        "toggle-overview",
				ScrollUp:   "focus-workspace-up",
				ScrollDown: "focus-workspace-down",
			},
		}},
		AppPresets:    map[string]string{},
		MetaStateFile: "/tmp/mxmouse-meta-state",
		WriteWaybar:   false,
	}
}

func DefaultConfigPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "mxmouse", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mxmouse", "config.json"), nil
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := cfg.Validate(); err != nil {
			return Config{}, fmt.Errorf("validate defaults: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("parse config %s: trailing JSON value", path)
		}
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported config version %d (expected %d)", c.Version, Version)
	}
	if c.Device.TriggerButton == "" {
		return errors.New("device.triggerButton is required")
	}
	if !c.Device.AutoDetect && c.Device.Path == "" {
		return errors.New("device.path is required when autoDetect is false")
	}
	if len(c.Presets) == 0 {
		return errors.New("at least one preset is required")
	}

	presets := make(map[string]Preset, len(c.Presets))
	for _, preset := range c.Presets {
		if preset.ID == "" || preset.Name == "" {
			return errors.New("every preset requires a non-empty id and name")
		}
		if _, exists := presets[preset.ID]; exists {
			return fmt.Errorf("duplicate preset id %q", preset.ID)
		}
		switch preset.Mode {
		case ModeGesture:
			if preset.Threshold <= 0 {
				return fmt.Errorf("preset %q threshold must be greater than zero", preset.ID)
			}
			if preset.Escalation < 0 {
				return fmt.Errorf("preset %q escalation cannot be negative", preset.ID)
			}
		case ModeMetaToggle:
			if preset.MetaKey == "" {
				return fmt.Errorf("preset %q metaKey is required in meta-toggle mode", preset.ID)
			}
			if !supportedMetaKey(preset.MetaKey) {
				return fmt.Errorf("preset %q uses unsupported metaKey %q", preset.ID, preset.MetaKey)
			}
		default:
			return fmt.Errorf("preset %q has unsupported mode %q", preset.ID, preset.Mode)
		}
		presets[preset.ID] = preset
	}
	if _, ok := presets[c.DefaultPresetID]; !ok {
		return fmt.Errorf("default preset %q does not exist", c.DefaultPresetID)
	}
	for appID, presetID := range c.AppPresets {
		if appID == "" {
			return errors.New("app preset assignments require a non-empty app_id")
		}
		if _, ok := presets[presetID]; !ok {
			return fmt.Errorf("app_id %q refers to unknown preset %q", appID, presetID)
		}
	}
	if c.WriteWaybar && c.MetaStateFile == "" {
		return errors.New("metaStateFile is required when writeWaybar is enabled")
	}
	return nil
}

func supportedMetaKey(name string) bool {
	switch name {
	case "KEY_LEFTMETA", "KEY_RIGHTMETA", "KEY_LEFTCTRL", "KEY_RIGHTCTRL",
		"KEY_LEFTALT", "KEY_RIGHTALT", "KEY_LEFTSHIFT", "KEY_RIGHTSHIFT":
		return true
	default:
		return false
	}
}

func (c Config) PresetForApp(appID string) Preset {
	presetID := c.AppPresets[appID]
	for _, preset := range c.Presets {
		if preset.ID == presetID {
			return preset
		}
	}
	for _, preset := range c.Presets {
		if preset.ID == c.DefaultPresetID {
			return preset
		}
	}
	return c.Presets[0]
}
