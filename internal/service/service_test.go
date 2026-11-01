package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/holoplot/go-evdev"

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
func TestShouldSuppressTriggerOnlyForUnappliedGeneration(t *testing.T) {
	tests := []struct {
		generation        int
		appliedGeneration int
		want              bool
	}{
		{generation: 0, appliedGeneration: 0, want: false},
		{generation: 4, appliedGeneration: 4, want: false},
		{generation: 5, appliedGeneration: 4, want: true},
	}
	for _, test := range tests {
		if got := shouldSuppressTrigger(test.generation, test.appliedGeneration); got != test.want {
			t.Errorf(
				"shouldSuppressTrigger(%d, %d) = %t, want %t",
				test.generation,
				test.appliedGeneration,
				got,
				test.want,
			)
		}
	}
}

func TestRetryDPIApplyRetriesUntilSuccess(t *testing.T) {
	calls := 0
	err := retryDPIApply(func() error {
		calls++
		if calls < 3 {
			return os.ErrDeadlineExceeded
		}
		return nil
	}, 3, 0)
	if err != nil {
		t.Fatalf("retryDPIApply returned error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("apply called %d times, want 3", calls)
	}
}

func TestRetryDPIApplyReturnsLastError(t *testing.T) {
	calls := 0
	err := retryDPIApply(func() error {
		calls++
		return os.ErrDeadlineExceeded
	}, 2, 0)
	if err != os.ErrDeadlineExceeded {
		t.Fatalf("retryDPIApply error = %v, want deadline exceeded", err)
	}
	if calls != 2 {
		t.Fatalf("apply called %d times, want 2", calls)
	}
}

func TestAppliedGenerationPersistsAndLoads(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	statusPath := filepath.Join(dir, "status.json")

	if got := previousAppliedGeneration(configPath, statusPath); got != 0 {
		t.Fatalf("initial applied generation = %d, want 0", got)
	}
	if err := storeAppliedGeneration(configPath, 7); err != nil {
		t.Fatal(err)
	}
	if got := previousAppliedGeneration(configPath, statusPath); got != 7 {
		t.Fatalf("persisted applied generation = %d, want 7", got)
	}
}

func TestWheelScaleSelectsAxesAndResolution(t *testing.T) {
	deviceSettings := config.Defaults().Device
	deviceSettings.VerticalScroll = 75
	deviceSettings.HorizontalScroll = 150
	for code, want := range map[evdev.EvCode]int{
		evdev.EvCode(0x08): 75,
		evdev.EvCode(0x0b): 75,
		evdev.EvCode(0x06): 150,
		evdev.EvCode(0x0c): 150,
		evdev.EvCode(0x09): 0,
	} {
		got, ok := wheelScale(code, deviceSettings)
		if want == 0 {
			if ok {
				t.Errorf("wheelScale(%#x) = %d, true; want false", code, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("wheelScale(%#x) = %d, %t; want %d, true", code, got, ok, want)
		}
	}
}

func TestScaleWheelValuePreservesFractionalCountsAndSigns(t *testing.T) {
	remainders := map[evdev.EvCode]float64{}
	const wheel = evdev.EvCode(0x0b)
	var got []int32
	for _, value := range []int32{120, 120, -120, -120} {
		got = append(got, scaleWheelValue(wheel, value, 25, remainders))
	}
	want := []int32{30, 30, -30, -30}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("scaled values = %v, want %v", got, want)
		}
	}
}

func TestScaleWheelValueAccumulatesSmallEvents(t *testing.T) {
	remainders := map[evdev.EvCode]float64{}
	const wheel = evdev.EvCode(0x08)
	got := []int32{
		scaleWheelValue(wheel, 1, 25, remainders),
		scaleWheelValue(wheel, 1, 25, remainders),
		scaleWheelValue(wheel, 1, 25, remainders),
		scaleWheelValue(wheel, 1, 25, remainders),
	}
	want := []int32{0, 0, 0, 1}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("scaled values = %v, want %v", got, want)
		}
	}
}
