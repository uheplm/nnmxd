package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/holoplot/go-evdev"

	"github.com/uheplm/nnmxd/internal/config"
	"github.com/uheplm/nnmxd/internal/gesture"
	"github.com/uheplm/nnmxd/internal/niri"
)

func TestGestureStatusIsPublished(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	want := GestureStatus{
		Active: true, XProgress: 0.4, XDirection: "left", YProgress: 0.2, YDirection: "down",
		TriggerCount: 3, TriggeredAxis: "x", TriggeredDirection: "left", TriggeredUnixMilli: 1234,
	}
	report := reporter{path: path, data: Status{Gesture: want}}
	report.update()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Gesture != want {
		t.Fatalf("published gesture status = %+v, want %+v", got.Gesture, want)
	}
}

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

func TestScaledGestureDistances(t *testing.T) {
	preset := config.Preset{
		Threshold:         101,
		RetriggerDistance: 81,
		Escalation:        21,
	}
	geometry := niri.WindowGeometry{
		WindowWidth: 960, WindowHeight: 540,
		OutputWidth: 1920, OutputHeight: 1080,
	}
	tests := []struct {
		name string
		mode config.WindowSizeScaling
		want config.GestureDistances
	}{
		{
			name: "disabled",
			mode: config.WindowSizeScalingDisabled,
			want: config.GestureDistances{Threshold: 101, RetriggerDistance: 81, Escalation: 21},
		},
		{
			name: "initial threshold",
			mode: config.WindowSizeScalingThreshold,
			want: config.GestureDistances{Threshold: 51, RetriggerDistance: 81, Escalation: 21},
		},
		{
			name: "retrigger and escalation",
			mode: config.WindowSizeScalingRetrigger,
			want: config.GestureDistances{Threshold: 101, RetriggerDistance: 41, Escalation: 11},
		},
		{
			name: "all",
			mode: config.WindowSizeScalingAll,
			want: config.GestureDistances{Threshold: 51, RetriggerDistance: 41, Escalation: 11},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preset.WindowSizeScaling = test.mode
			xAxis, yAxis := scaledGestureDistances(preset, geometry)
			if xAxis != test.want || yAxis != test.want {
				t.Fatalf("scaled distances = (%#v, %#v), want (%#v, %#v)",
					xAxis, yAxis, test.want, test.want)
			}
		})
	}
}

func TestScaledGestureDistancesUsesIndependentAxesAndGeometry(t *testing.T) {
	preset := config.Preset{
		WindowSizeScaling:    config.WindowSizeScalingAll,
		SeparateAxisSettings: true,
		XAxis: config.GestureDistances{
			Threshold: 100, RetriggerDistance: 80, Escalation: 20,
		},
		YAxis: config.GestureDistances{
			Threshold: 60, RetriggerDistance: 40, Escalation: 10,
		},
	}
	geometry := niri.WindowGeometry{
		WindowWidth: 960, WindowHeight: 810,
		OutputWidth: 1920, OutputHeight: 1080,
	}
	xAxis, yAxis := scaledGestureDistances(preset, geometry)
	if xAxis != (config.GestureDistances{Threshold: 50, RetriggerDistance: 40, Escalation: 10}) {
		t.Fatalf("X distances = %#v", xAxis)
	}
	if yAxis != (config.GestureDistances{Threshold: 45, RetriggerDistance: 30, Escalation: 8}) {
		t.Fatalf("Y distances = %#v", yAxis)
	}
}

func TestScaledGestureDistancesFallbackForInvalidGeometry(t *testing.T) {
	preset := config.Preset{
		Threshold:         100,
		RetriggerDistance: 120,
		Escalation:        20,
		WindowSizeScaling: config.WindowSizeScalingAll,
	}
	xAxis, yAxis := scaledGestureDistances(
		preset,
		niri.WindowGeometry{WindowWidth: 0, WindowHeight: 0, OutputWidth: 1920, OutputHeight: 1080},
	)
	want := config.GestureDistances{Threshold: 100, RetriggerDistance: 120, Escalation: 20}
	if xAxis != want || yAxis != want {
		t.Fatalf("invalid geometry distances = (%#v, %#v), want (%#v, %#v)", xAxis, yAxis, want, want)
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

func TestWindowWidthAdjustedWheelScale(t *testing.T) {
	geometry := niri.WindowGeometry{WindowWidth: 960, OutputWidth: 1920}
	tests := []struct {
		name     string
		code     evdev.EvCode
		mode     config.HorizontalScrollWindowWidthScaling
		geometry niri.WindowGeometry
		want     float64
	}{
		{
			name:     "disabled",
			code:     evdev.EvCode(0x06),
			mode:     config.HorizontalScrollWindowWidthScalingDisabled,
			geometry: geometry,
			want:     100,
		},
		{
			name:     "direct horizontal",
			code:     evdev.EvCode(0x06),
			mode:     config.HorizontalScrollWindowWidthScalingDirect,
			geometry: geometry,
			want:     50,
		},
		{
			name:     "reverse horizontal",
			code:     evdev.EvCode(0x0c),
			mode:     config.HorizontalScrollWindowWidthScalingReverse,
			geometry: geometry,
			want:     200,
		},
		{
			name:     "vertical unaffected",
			code:     evdev.EvCode(0x08),
			mode:     config.HorizontalScrollWindowWidthScalingReverse,
			geometry: geometry,
			want:     100,
		},
		{
			name:     "missing geometry preserves base scale",
			code:     evdev.EvCode(0x06),
			mode:     config.HorizontalScrollWindowWidthScalingReverse,
			geometry: niri.WindowGeometry{},
			want:     100,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := windowWidthAdjustedWheelScale(test.code, 100, test.mode, test.geometry)
			if got != test.want {
				t.Fatalf("adjusted scale = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProfileDPIAndScrollSettingsFallBackToDeviceValues(t *testing.T) {
	deviceSettings := config.Device{DPI: 800, VerticalScroll: 75, HorizontalScroll: 140}
	preset := config.Preset{}
	if got := effectiveDPI(preset, deviceSettings); got != 800 {
		t.Fatalf("fallback DPI = %d, want 800", got)
	}
	if got := effectiveScrollSettings(preset, deviceSettings); got != deviceSettings {
		t.Fatalf("fallback scroll settings = %#v, want %#v", got, deviceSettings)
	}

	preset.DPI = 1200
	preset.VerticalScroll = 60
	preset.HorizontalScroll = 200
	if got := effectiveDPI(preset, deviceSettings); got != 1200 {
		t.Fatalf("profile DPI = %d, want 1200", got)
	}
	want := deviceSettings
	want.VerticalScroll = 60
	want.HorizontalScroll = 200
	if got := effectiveScrollSettings(preset, deviceSettings); got != want {
		t.Fatalf("profile scroll settings = %#v, want %#v", got, want)
	}
}

func TestNeedsBaselineDPIOnlyWhenProfileOverridesDeviceUnchangedSetting(t *testing.T) {
	cfg := config.Defaults()
	if needsBaselineDPI(cfg) {
		t.Fatal("baseline DPI should not be needed when all settings inherit device defaults")
	}
	cfg.Presets[0].DPI = 1200
	if !needsBaselineDPI(cfg) {
		t.Fatal("baseline DPI should be captured when a profile overrides an unchanged device DPI")
	}
	cfg.Device.DPI = 800
	if needsBaselineDPI(cfg) {
		t.Fatal("baseline DPI should not be needed when device DPI provides the fallback")
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

func TestHorizontalWheelDirection(t *testing.T) {
	tests := []struct {
		code      evdev.EvCode
		value     int32
		want      gesture.InputKind
		wantValid bool
	}{
		{code: evdev.REL_HWHEEL, value: -1, want: gesture.ThumbwheelLeft, wantValid: true},
		{code: evdev.REL_HWHEEL_HI_RES, value: 120, want: gesture.ThumbwheelRight, wantValid: true},
		{code: evdev.REL_HWHEEL, value: 0, wantValid: false},
		{code: evdev.REL_WHEEL, value: 1, wantValid: false},
	}
	for _, test := range tests {
		got, ok := horizontalWheelDirection(test.code, test.value)
		if got != test.want || ok != test.wantValid {
			t.Errorf("horizontalWheelDirection(%#x, %d) = (%d, %t), want (%d, %t)",
				test.code, test.value, got, ok, test.want, test.wantValid)
		}
	}
	if !isLegacyHighResolutionWheel(evdev.REL_HWHEEL) {
		t.Fatal("REL_HWHEEL was not recognized as the legacy horizontal-wheel event")
	}
	if isLegacyHighResolutionWheel(evdev.REL_HWHEEL_HI_RES) {
		t.Fatal("REL_HWHEEL_HI_RES was incorrectly recognized as a legacy event")
	}
}

func TestThumbwheelActionSelectsHeldAndUnheldBindings(t *testing.T) {
	left := config.Action{Type: config.ActionNiri, Value: "left"}
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	heldLeft := config.Action{Type: config.ActionNoctalia, Value: "audio mic mute"}
	preset := config.Preset{Bindings: config.Bindings{
		ThumbwheelLeft: left, ThumbwheelRight: right, ThumbwheelButtonLeft: heldLeft,
	}}
	tests := []struct {
		name string
		dir  gesture.InputKind
		held bool
		want config.Action
	}{
		{name: "unheld left", dir: gesture.ThumbwheelLeft, want: left},
		{name: "unheld right", dir: gesture.ThumbwheelRight, want: right},
		{name: "held left", dir: gesture.ThumbwheelLeft, held: true, want: heldLeft},
		{name: "unassigned held right", dir: gesture.ThumbwheelRight, held: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := thumbwheelAction(preset, test.dir, test.held); got != test.want {
				t.Fatalf("thumbwheelAction() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestThumbwheelActionCountAccumulatesHighResolutionDetents(t *testing.T) {
	remainders := map[thumbwheelRemainderKey]int64{}
	key := thumbwheelRemainderKey{code: evdev.REL_HWHEEL_HI_RES, presetID: "default"}
	for index, value := range []int32{40, 40, 40, -120} {
		count, direction := thumbwheelActionCount(evdev.REL_HWHEEL_HI_RES, value, remainders, key)
		wantCount := 0
		if index == 2 || index == 3 {
			wantCount = 1
		}
		wantDirection := gesture.ThumbwheelRight
		if index == 3 {
			wantDirection = gesture.ThumbwheelLeft
		}
		if count != wantCount || (count > 0 && direction != wantDirection) {
			t.Errorf("event %d (%d): got (%d, %d), want (%d, %d)",
				index, value, count, direction, wantCount, wantDirection)
		}
	}
}

func TestThumbwheelRemaindersAreScopedToPresetAndButtonState(t *testing.T) {
	remainders := map[thumbwheelRemainderKey]int64{}
	const code evdev.EvCode = evdev.REL_HWHEEL_HI_RES
	defaultKey := thumbwheelRemainderKey{code: code, presetID: "default"}
	discordKey := thumbwheelRemainderKey{code: code, presetID: "discord"}
	heldKey := thumbwheelRemainderKey{code: code, presetID: "discord", held: true}

	if count, _ := thumbwheelActionCount(code, 80, remainders, defaultKey); count != 0 {
		t.Fatalf("partial default-profile detent count = %d, want 0", count)
	}
	if count, _ := thumbwheelActionCount(code, 80, remainders, discordKey); count != 0 {
		t.Fatalf("partial Discord detent count = %d, want 0", count)
	}
	if count, _ := thumbwheelActionCount(code, 40, remainders, heldKey); count != 0 {
		t.Fatalf("partial held detent count = %d, want 0", count)
	}
	if count, _ := thumbwheelActionCount(code, 40, remainders, discordKey); count != 1 {
		t.Fatalf("Discord accumulated detent count = %d, want 1", count)
	}
	if count, _ := thumbwheelActionCount(code, 40, remainders, heldKey); count != 0 {
		t.Fatalf("held accumulated detent count = %d, want 0", count)
	}
}

func TestLegacyThumbwheelEventsCountWheelSteps(t *testing.T) {
	remainders := map[thumbwheelRemainderKey]int64{}
	key := thumbwheelRemainderKey{code: evdev.REL_HWHEEL}
	count, direction := thumbwheelActionCount(evdev.REL_HWHEEL, -2, remainders, key)
	if count != 2 || direction != gesture.ThumbwheelLeft {
		t.Fatalf("legacy wheel action = (%d, %d), want (2, %d)", count, direction, gesture.ThumbwheelLeft)
	}
}

func TestSideButtonActionMapsPhysicalButtonsToProfileBindings(t *testing.T) {
	preset := config.Preset{Bindings: config.Bindings{
		SideBack:    config.Action{Type: config.ActionNiri, Value: "focus-column-left"},
		SideForward: config.Action{Type: config.ActionNoctalia, Value: "panel-toggle"},
	}}
	tests := []struct {
		name string
		code evdev.EvCode
		want config.Action
	}{
		{name: "back side button", code: evdev.BTN_SIDE, want: preset.Bindings.SideBack},
		{name: "front side button", code: evdev.BTN_EXTRA, want: preset.Bindings.SideForward},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := sideButtonAction(preset, test.code)
			if !ok || got != test.want {
				t.Fatalf("sideButtonAction() = (%#v, %t), want (%#v, true)", got, ok, test.want)
			}
		})
	}
	if _, ok := sideButtonAction(preset, evdev.BTN_TASK); ok {
		t.Fatal("under-wheel button should not have a profile action")
	}
}

func TestHandleSideButtonEventDispatchesOnceAndConsumesRelease(t *testing.T) {
	action := config.Action{Type: config.ActionNiri, Value: "focus-column-left"}
	preset := config.Preset{Bindings: config.Bindings{SideBack: action}}
	consumed := map[evdev.EvCode]bool{}
	dispatched := 0
	dispatch := func(got config.Action) {
		if got != action {
			t.Errorf("dispatched action = %#v, want %#v", got, action)
		}
		dispatched++
	}
	press := &evdev.InputEvent{Type: evdev.EV_KEY, Code: evdev.BTN_SIDE, Value: 1}
	repeat := &evdev.InputEvent{Type: evdev.EV_KEY, Code: evdev.BTN_SIDE, Value: 2}
	release := &evdev.InputEvent{Type: evdev.EV_KEY, Code: evdev.BTN_SIDE, Value: 0}
	for _, event := range []*evdev.InputEvent{press, repeat, release} {
		if !handleSideButtonEvent(preset, event, consumed, dispatch) {
			t.Errorf("event value %d was not consumed", event.Value)
		}
	}
	if dispatched != 1 {
		t.Fatalf("dispatch count = %d, want 1", dispatched)
	}
	if consumed[evdev.BTN_SIDE] {
		t.Fatal("button remained marked consumed after release")
	}
}

func TestHandleSideButtonEventPassesUnassignedEventsThrough(t *testing.T) {
	preset := config.Preset{}
	consumed := map[evdev.EvCode]bool{}
	dispatch := func(config.Action) { t.Error("dispatched an unassigned button") }
	for _, event := range []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.BTN_SIDE, Value: 1},
		{Type: evdev.EV_KEY, Code: evdev.BTN_TASK, Value: 1},
		{Type: evdev.EV_REL, Code: evdev.REL_X, Value: 1},
	} {
		if handleSideButtonEvent(preset, event, consumed, dispatch) {
			t.Errorf("event type %d code %#x was consumed without an assigned action", event.Type, event.Code)
		}
	}
}

func TestThumbClickActionMapsLeftMiddleAndRightButtons(t *testing.T) {
	left := config.Action{Type: config.ActionNiri, Value: "maximize-column"}
	middle := config.Action{Type: config.ActionCommand, Value: "notify-send middle"}
	right := config.Action{Type: config.ActionNoctalia, Value: "panel-toggle"}
	preset := config.Preset{Bindings: config.Bindings{
		ThumbLeftClick: left, ThumbMiddleClick: middle, ThumbRightClick: right,
	}}
	for _, test := range []struct {
		code evdev.EvCode
		want config.Action
	}{
		{code: evdev.BTN_LEFT, want: left},
		{code: evdev.BTN_MIDDLE, want: middle},
		{code: evdev.BTN_RIGHT, want: right},
	} {
		got, ok := thumbClickAction(preset, test.code)
		if !ok || got != test.want {
			t.Errorf("thumbClickAction(%#x) = (%#v, %t), want (%#v, true)",
				test.code, got, ok, test.want)
		}
	}
	if _, ok := thumbClickAction(preset, evdev.BTN_SIDE); ok {
		t.Fatal("side button should not have a thumb-click binding")
	}
}

func TestHandleThumbMiddleClickEventDispatchesOnceAndConsumesRelease(t *testing.T) {
	action := config.Action{Type: config.ActionNiri, Value: "maximize-column"}
	preset := config.Preset{Bindings: config.Bindings{ThumbMiddleClick: action}}
	consumed := map[evdev.EvCode]bool{}
	dispatched := 0
	dispatch := func(got config.Action) {
		if got != action {
			t.Errorf("dispatched action = %#v, want %#v", got, action)
		}
		dispatched++
	}
	events := []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.BTN_MIDDLE, Value: 1},
		{Type: evdev.EV_KEY, Code: evdev.BTN_MIDDLE, Value: 2},
		{Type: evdev.EV_KEY, Code: evdev.BTN_MIDDLE, Value: 0},
	}
	for _, event := range events {
		if !handleThumbClickEvent(preset, event, true, consumed, dispatch) {
			t.Errorf("active chord event value %d was not consumed", event.Value)
		}
	}
	if dispatched != 1 {
		t.Fatalf("dispatch count = %d, want 1", dispatched)
	}
	if consumed[evdev.BTN_MIDDLE] {
		t.Fatal("middle click remained consumed after release")
	}
}

func TestHandleThumbClickEventPassesThroughUnassignedAndInactiveClicks(t *testing.T) {
	consumed := map[evdev.EvCode]bool{}
	dispatch := func(config.Action) { t.Error("dispatched an unassigned or inactive click") }
	leftPress := &evdev.InputEvent{Type: evdev.EV_KEY, Code: evdev.BTN_LEFT, Value: 1}
	for _, test := range []struct {
		name   string
		preset config.Preset
		active bool
	}{
		{name: "unassigned active chord", preset: config.Preset{}, active: true},
		{
			name: "assigned click without thumb gesture",
			preset: config.Preset{Bindings: config.Bindings{
				ThumbLeftClick: config.Action{Type: config.ActionNiri, Value: "maximize-column"},
			}},
			active: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if handleThumbClickEvent(test.preset, leftPress, test.active, consumed, dispatch) {
				t.Fatal("click was consumed without an assigned active chord")
			}
		})
	}
}
