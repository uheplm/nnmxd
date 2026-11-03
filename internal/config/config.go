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

type WindowSizeScaling string

const (
	WindowSizeScalingDisabled  WindowSizeScaling = "disabled"
	WindowSizeScalingThreshold WindowSizeScaling = "threshold"
	WindowSizeScalingRetrigger WindowSizeScaling = "retrigger"
	WindowSizeScalingAll       WindowSizeScaling = "all"
)

type Device struct {
	AutoDetect       bool   `json:"autoDetect"`
	Path             string `json:"path"`
	TriggerButton    string `json:"triggerButton"`
	DPI              int    `json:"dpi"`
	VerticalScroll   int    `json:"verticalScroll"`
	HorizontalScroll int    `json:"horizontalScroll"`
}

type ActionType string

const (
	ActionNiri     ActionType = "niri"
	ActionNoctalia ActionType = "noctalia"
	ActionCommand  ActionType = "command"
)

type Action struct {
	Type      ActionType `json:"type"`
	Value     string     `json:"value"`
	Arguments string     `json:"arguments,omitempty"`
}

func (a *Action) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var legacy string
		if err := json.Unmarshal(data, &legacy); err != nil {
			return err
		}
		*a = Action{Type: ActionNiri, Value: legacy}
		return nil
	}

	type actionJSON Action
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded actionJSON
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if decoded.Type == "" {
		decoded.Type = ActionNiri
	}
	*a = Action(decoded)
	return nil
}

type Bindings struct {
	Left                  Action `json:"left"`
	Right                 Action `json:"right"`
	Up                    Action `json:"up"`
	Down                  Action `json:"down"`
	Tap                   Action `json:"tap"`
	ScrollUp              Action `json:"scrollUp"`
	ScrollDown            Action `json:"scrollDown"`
	ThumbwheelLeft        Action `json:"thumbwheelLeft"`
	ThumbwheelRight       Action `json:"thumbwheelRight"`
	ThumbwheelButtonLeft  Action `json:"thumbwheelButtonLeft"`
	ThumbwheelButtonRight Action `json:"thumbwheelButtonRight"`
	SideBack              Action `json:"sideBack"`
	SideForward           Action `json:"sideForward"`
	ThumbLeftClick        Action `json:"thumbLeftClick"`
	ThumbMiddleClick      Action `json:"thumbMiddleClick"`
	ThumbRightClick       Action `json:"thumbRightClick"`
}

type GestureDistances struct {
	Threshold         int `json:"threshold"`
	RetriggerDistance int `json:"retriggerDistance"`
	Escalation        int `json:"escalation"`
}

type Preset struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Mode                 Mode              `json:"mode"`
	Glyph                string            `json:"glyph,omitempty"`
	DPI                  int               `json:"dpi"`
	VerticalScroll       int               `json:"verticalScroll"`
	HorizontalScroll     int               `json:"horizontalScroll"`
	Threshold            int               `json:"threshold"`
	RetriggerDistance    int               `json:"retriggerDistance"`
	Escalation           int               `json:"escalation"`
	SeparateAxisSettings bool              `json:"separateAxisSettings"`
	XAxis                GestureDistances  `json:"xAxis"`
	YAxis                GestureDistances  `json:"yAxis"`
	WindowSizeScaling    WindowSizeScaling `json:"windowSizeScaling"`
	MetaKey              string            `json:"metaKey"`
	Bindings             Bindings          `json:"bindings"`
}

type Config struct {
	Version         int               `json:"version"`
	ApplyGeneration int               `json:"applyGeneration"`
	Device          Device            `json:"device"`
	DefaultPresetID string            `json:"defaultPresetId"`
	Presets         []Preset          `json:"presets"`
	AppPresets      map[string]string `json:"appPresets"`
}

type configFile struct {
	Config
	MetaStateFile string `json:"metaStateFile"`
	WriteWaybar   bool   `json:"writeWaybar"`
}

func Defaults() Config {
	return Config{
		Version: Version,
		Device: Device{
			AutoDetect:       true,
			TriggerButton:    "BTN_FORWARD",
			VerticalScroll:   100,
			HorizontalScroll: 100,
		},
		DefaultPresetID: "default",
		Presets: []Preset{{
			ID:                "default",
			Name:              "Default",
			Mode:              ModeGesture,
			Glyph:             "arrows-move",
			Threshold:         100,
			RetriggerDistance: 100,
			Escalation:        0,
			XAxis:             GestureDistances{Threshold: 100, RetriggerDistance: 100},
			YAxis:             GestureDistances{Threshold: 100, RetriggerDistance: 100},
			WindowSizeScaling: WindowSizeScalingDisabled,
			MetaKey:           "KEY_LEFTMETA",
			Bindings: Bindings{
				Left:             Action{Type: ActionNiri, Value: "focus-column-left"},
				Right:            Action{Type: ActionNiri, Value: "focus-column-right"},
				Up:               Action{Type: ActionNiri, Value: "focus-workspace-up"},
				Down:             Action{Type: ActionNiri, Value: "focus-workspace-down"},
				Tap:              Action{Type: ActionNiri, Value: "toggle-overview"},
				ScrollUp:         Action{Type: ActionNiri, Value: "focus-workspace-up"},
				ScrollDown:       Action{Type: ActionNiri, Value: "focus-workspace-down"},
				SideBack:         Action{Type: ActionNiri},
				SideForward:      Action{Type: ActionNiri},
				ThumbLeftClick:   Action{Type: ActionNiri},
				ThumbMiddleClick: Action{Type: ActionNiri},
				ThumbRightClick:  Action{Type: ActionNiri},
			},
		}},
		AppPresets: map[string]string{},
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
	file := configFile{Config: cfg}
	if err := decoder.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("parse config %s: trailing JSON value", path)
		}
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg = file.Config
	var rawConfig struct {
		Presets []map[string]json.RawMessage `json:"presets"`
	}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	for i := range cfg.Presets {
		if i < len(rawConfig.Presets) {
			if _, exists := rawConfig.Presets[i]["retriggerDistance"]; !exists {
				cfg.Presets[i].RetriggerDistance = cfg.Presets[i].Threshold
			}
			migrateAxisDistances(&cfg.Presets[i], rawConfig.Presets[i], "xAxis")
			migrateAxisDistances(&cfg.Presets[i], rawConfig.Presets[i], "yAxis")
		}
		if cfg.Presets[i].WindowSizeScaling == "" {
			cfg.Presets[i].WindowSizeScaling = WindowSizeScalingDisabled
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

func migrateAxisDistances(preset *Preset, raw map[string]json.RawMessage, field string) {
	axis := &preset.XAxis
	if field == "yAxis" {
		axis = &preset.YAxis
	}
	rawAxis, exists := raw[field]
	if !exists {
		*axis = GestureDistances{
			Threshold:         preset.Threshold,
			RetriggerDistance: preset.RetriggerDistance,
			Escalation:        preset.Escalation,
		}
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawAxis, &fields); err != nil {
		return
	}
	if _, exists := fields["threshold"]; !exists {
		axis.Threshold = preset.Threshold
	}
	if _, exists := fields["retriggerDistance"]; !exists {
		axis.RetriggerDistance = preset.RetriggerDistance
	}
	if _, exists := fields["escalation"]; !exists {
		axis.Escalation = preset.Escalation
	}
}

func (c Config) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported config version %d (expected %d)", c.Version, Version)
	}
	if c.ApplyGeneration < 0 {
		return errors.New("applyGeneration cannot be negative")
	}
	if c.Device.TriggerButton == "" {
		return errors.New("device.triggerButton is required")
	}
	if !c.Device.AutoDetect && c.Device.Path == "" {
		return errors.New("device.path is required when autoDetect is false")
	}
	if c.Device.DPI != 0 && (c.Device.DPI < 100 || c.Device.DPI > 16000) {
		return errors.New("device.dpi must be 0 (unchanged) or between 100 and 16000")
	}
	if c.Device.VerticalScroll < 10 || c.Device.VerticalScroll > 500 {
		return errors.New("device.verticalScroll must be between 10 and 500")
	}
	if c.Device.HorizontalScroll < 10 || c.Device.HorizontalScroll > 500 {
		return errors.New("device.horizontalScroll must be between 10 and 500")
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
		switch preset.WindowSizeScaling {
		case "", WindowSizeScalingDisabled, WindowSizeScalingThreshold,
			WindowSizeScalingRetrigger, WindowSizeScalingAll:
		default:
			return fmt.Errorf("preset %q has unsupported windowSizeScaling %q", preset.ID, preset.WindowSizeScaling)
		}
		if preset.DPI != 0 && (preset.DPI < 100 || preset.DPI > 16000) {
			return fmt.Errorf("preset %q dpi must be 0 (device default) or between 100 and 16000", preset.ID)
		}
		if preset.VerticalScroll != 0 && (preset.VerticalScroll < 10 || preset.VerticalScroll > 500) {
			return fmt.Errorf("preset %q verticalScroll must be 0 (device default) or between 10 and 500", preset.ID)
		}
		if preset.HorizontalScroll != 0 && (preset.HorizontalScroll < 10 || preset.HorizontalScroll > 500) {
			return fmt.Errorf("preset %q horizontalScroll must be 0 (device default) or between 10 and 500", preset.ID)
		}
		for name, binding := range map[string]Action{
			"sideBack": preset.Bindings.SideBack, "sideForward": preset.Bindings.SideForward,
		} {
			if err := binding.Validate(); err != nil {
				return fmt.Errorf("preset %q binding %s: %w", preset.ID, name, err)
			}
		}
		switch preset.Mode {
		case ModeGesture:
			if preset.Threshold <= 0 {
				return fmt.Errorf("preset %q threshold must be greater than zero", preset.ID)
			}
			if preset.RetriggerDistance <= 0 {
				return fmt.Errorf("preset %q retriggerDistance must be greater than zero", preset.ID)
			}
			if preset.Escalation < 0 {
				return fmt.Errorf("preset %q escalation cannot be negative", preset.ID)
			}
			if preset.SeparateAxisSettings {
				for name, distances := range map[string]GestureDistances{
					"xAxis": preset.XAxis, "yAxis": preset.YAxis,
				} {
					if distances.Threshold <= 0 {
						return fmt.Errorf("preset %q %s threshold must be greater than zero", preset.ID, name)
					}
					if distances.RetriggerDistance <= 0 {
						return fmt.Errorf("preset %q %s retriggerDistance must be greater than zero", preset.ID, name)
					}
					if distances.Escalation < 0 {
						return fmt.Errorf("preset %q %s escalation cannot be negative", preset.ID, name)
					}
				}
			}
			for name, binding := range map[string]Action{
				"left": preset.Bindings.Left, "right": preset.Bindings.Right,
				"up": preset.Bindings.Up, "down": preset.Bindings.Down,
				"tap": preset.Bindings.Tap, "scrollUp": preset.Bindings.ScrollUp,
				"scrollDown":            preset.Bindings.ScrollDown,
				"thumbwheelLeft":        preset.Bindings.ThumbwheelLeft,
				"thumbwheelRight":       preset.Bindings.ThumbwheelRight,
				"thumbwheelButtonLeft":  preset.Bindings.ThumbwheelButtonLeft,
				"thumbwheelButtonRight": preset.Bindings.ThumbwheelButtonRight,
				"thumbLeftClick":        preset.Bindings.ThumbLeftClick,
				"thumbMiddleClick":      preset.Bindings.ThumbMiddleClick,
				"thumbRightClick":       preset.Bindings.ThumbRightClick,
			} {
				if err := binding.Validate(); err != nil {
					return fmt.Errorf("preset %q binding %s: %w", preset.ID, name, err)
				}
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
	return nil
}

func (a Action) Validate() error {
	switch a.Type {
	case "", ActionNiri, ActionNoctalia, ActionCommand:
	default:
		return fmt.Errorf("unsupported action type %q", a.Type)
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
