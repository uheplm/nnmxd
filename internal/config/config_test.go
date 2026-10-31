package config

import (
	"os"
	"path/filepath"
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

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultPresetID != "default" {
		t.Fatalf("default preset = %q", cfg.DefaultPresetID)
	}
}
