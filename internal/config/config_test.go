package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultsValidate(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Defaults() failed Validate: %v", err)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := writeFile(path, `
mode: meta-toggle
device:
  auto_detect: true
  trigger_button: BTN_FORWARD
meta_toggle:
  key: KEY_LEFTMETA
  state_file: /tmp/x-state
  waybar: true
`); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeMetaToggle {
		t.Errorf("mode = %q, want %q", cfg.Mode, ModeMetaToggle)
	}
	// Defaults should still fill unspecified gesture fields, even though
	// meta-toggle mode doesn't use them.
	if cfg.Gesture.Threshold != 100 {
		t.Errorf("Gesture.Threshold = %d, want default 100", cfg.Gesture.Threshold)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "unknown mode",
			mutate:  func(c *Config) { c.Mode = "nope" },
			wantErr: "unknown mode",
		},
		{
			name:    "empty trigger",
			mutate:  func(c *Config) { c.Device.TriggerButton = "" },
			wantErr: "trigger_button is required",
		},
		{
			name: "manual device without path",
			mutate: func(c *Config) {
				c.Device.AutoDetect = false
				c.Device.Path = ""
			},
			wantErr: "device.path is required",
		},
		{
			name: "gesture zero threshold",
			mutate: func(c *Config) {
				c.Mode = ModeGesture
				c.Gesture.Threshold = 0
			},
			wantErr: "threshold must be > 0",
		},
		{
			name: "meta-toggle waybar without state file",
			mutate: func(c *Config) {
				c.Mode = ModeMetaToggle
				c.MetaToggle.Waybar = true
				c.MetaToggle.StateFile = ""
			},
			wantErr: "state_file is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate returned nil, want error")
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	// Empty path + no file in search paths in test env (XDG_CONFIG_HOME set to
	// a nonexistent dir) should still succeed using built-in defaults.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "nope"))
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeGesture {
		t.Errorf("Mode = %q, want default %q", cfg.Mode, ModeGesture)
	}
}

func writeFile(path, content string) error {
	return osWriteFile(path, []byte(content), 0o644)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
