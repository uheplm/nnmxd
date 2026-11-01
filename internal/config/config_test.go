package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsValidateAndSelectAppPreset(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	if cfg.Presets[0].Threshold != 100 || cfg.Presets[0].RetriggerDistance != 100 || cfg.Presets[0].Escalation != 0 {
		t.Fatalf("default gesture distances = (%d, %d, %d), want (100, 100, 0)",
			cfg.Presets[0].Threshold, cfg.Presets[0].RetriggerDistance, cfg.Presets[0].Escalation)
	}
	if got := cfg.PresetForApp("org.example.App").ID; got != "default" {
		t.Fatalf("fallback preset = %q, want default", got)
	}

	cfg.Presets = append(cfg.Presets, Preset{
		ID: "meta", Name: "Meta", Mode: ModeMetaToggle, MetaKey: "KEY_LEFTMETA",
	})
	cfg.AppPresets["org.example.App"] = "meta"
	if got := cfg.PresetForApp("org.example.App").ID; got != "meta" {
		t.Fatalf("assigned preset = %q, want meta", got)
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for name, contents := range map[string]string{
		"unknown":  `{"version":1,"surprise":true}`,
		"trailing": `{"version":1} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load succeeded; want an error")
			}
		})
	}
}

func TestValidationRejectsUnknownPresetAssignment(t *testing.T) {
	cfg := Defaults()
	cfg.AppPresets["org.example.App"] = "missing"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded with unknown preset assignment")
	}
}

func TestValidationRejectsNegativeApplyGeneration(t *testing.T) {
	cfg := Defaults()
	cfg.ApplyGeneration = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded with a negative apply generation")
	}
}

func TestValidationRejectsInvalidDPIAndScrollScales(t *testing.T) {
	tests := map[string]func(*Config){
		"dpi below supported range":     func(cfg *Config) { cfg.Device.DPI = 50 },
		"dpi above supported range":     func(cfg *Config) { cfg.Device.DPI = 16001 },
		"vertical scroll below range":   func(cfg *Config) { cfg.Device.VerticalScroll = 9 },
		"horizontal scroll above range": func(cfg *Config) { cfg.Device.HorizontalScroll = 501 },
	}
	for name, modify := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			modify(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate succeeded with invalid device settings")
			}
		})
	}
}

func TestValidationRejectsUnknownGestureActionType(t *testing.T) {
	cfg := Defaults()
	cfg.Presets[0].Bindings.Tap.Type = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded with an unknown gesture action type")
	}
}

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultPresetID != "default" {
		t.Fatalf("default preset = %q", cfg.DefaultPresetID)
	}
}

func TestLoadLegacyPresetDefaultsRetriggerDistanceToThreshold(t *testing.T) {
	cfg := Defaults()
	cfg.Presets[0].Threshold = 175
	cfg.Presets[0].Escalation = 0
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var presets []map[string]json.RawMessage
	if err := json.Unmarshal(document["presets"], &presets); err != nil {
		t.Fatal(err)
	}
	delete(presets[0], "retriggerDistance")
	document["presets"], err = json.Marshal(presets)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Presets[0].RetriggerDistance != 175 {
		t.Fatalf("legacy retrigger distance = %d, want threshold 175", loaded.Presets[0].RetriggerDistance)
	}
}

func TestLoadIgnoresLegacyWaybarFields(t *testing.T) {
	cfg := Defaults()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	contents := strings.TrimSpace(string(data))
	contents = strings.TrimSuffix(contents, "}") + `,"writeWaybar":true,"metaStateFile":"/tmp/legacy-state"}`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load rejected legacy Waybar settings: %v", err)
	}
	if loaded.Version != cfg.Version || loaded.DefaultPresetID != cfg.DefaultPresetID {
		t.Fatalf("legacy settings changed config: %#v", loaded)
	}
	loadedJSON := string(mustMarshal(t, loaded))
	if strings.Contains(loadedJSON, "Waybar") || strings.Contains(loadedJSON, "metaStateFile") {
		t.Fatal("legacy Waybar settings were retained in the loaded config")
	}
}

func TestLoadLegacyStringBindingsAsNiriActions(t *testing.T) {
	cfg := Defaults()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	presets := document["presets"].([]any)
	bindings := presets[0].(map[string]any)["bindings"].(map[string]any)
	bindings["scrollUp"] = "focus-workspace-up"
	contents, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Presets[0].Bindings.ScrollUp; got.Type != ActionNiri || got.Value != "focus-workspace-up" {
		t.Fatalf("legacy binding = %#v, want Niri focus-workspace-up", got)
	}
}

func TestLoadPreservesApplyGeneration(t *testing.T) {
	cfg := Defaults()
	cfg.ApplyGeneration = 3
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ApplyGeneration != 3 {
		t.Fatalf("apply generation = %d, want 3", loaded.ApplyGeneration)
	}
}

func TestLoadPreservesPresetGlyph(t *testing.T) {
	cfg := Defaults()
	cfg.Presets[0].Glyph = "device-gamepad-2"
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Presets[0].Glyph != "device-gamepad-2" {
		t.Fatalf("preset glyph = %q, want device-gamepad-2", loaded.Presets[0].Glyph)
	}
}

func TestLoadLegacyDeviceSettingsUseNeutralScalingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"device":{"autoDetect":true,"triggerButton":"BTN_FORWARD"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Device.DPI != 0 || cfg.Device.VerticalScroll != 100 || cfg.Device.HorizontalScroll != 100 {
		t.Fatalf("legacy device defaults = %#v", cfg.Device)
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
