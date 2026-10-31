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
	if strings.Contains(string(mustMarshal(t, loaded)), "Waybar") ||
		strings.Contains(string(mustMarshal(t, loaded)), "metaStateFile") {
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

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
