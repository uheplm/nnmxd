package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/uheplm/nnmxd/internal/config"
)

func TestWriteConfigValidatesAndAtomicallyPublishes(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidate.json")
	data, err := json.Marshal(config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "config", "config.json")
	if err := WriteConfig(target, candidatePath); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultPresetID != "default" {
		t.Fatalf("default preset = %q", loaded.DefaultPresetID)
	}
}

func TestWriteConfigDoesNotReplaceValidConfigWithInvalidCandidate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	original, err := json.Marshal(config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(candidate, []byte(`{"version":999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(target, candidate); err == nil {
		t.Fatal("invalid config was accepted")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("invalid candidate replaced existing config")
	}
}
