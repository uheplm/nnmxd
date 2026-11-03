package gesture

import (
	"reflect"
	"testing"

	"github.com/uheplm/nnmxd/internal/config"
)

func TestTapAndGesture(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "focus-column-right"}
	tap := config.Action{Type: config.ActionNiri, Value: "toggle-overview"}
	engine := New(100, 100, 0, Bindings{Right: right, Tap: tap})
	if got := engine.Step(Input{Kind: ButtonPress}); len(got) != 0 {
		t.Fatalf("press actions = %v", got)
	}
	if got := engine.Step(Input{Kind: ButtonRelease}); !reflect.DeepEqual(got, []config.Action{tap}) {
		t.Fatalf("tap = %v", got)
	}

	engine.Step(Input{Kind: ButtonPress})
	got := engine.Step(Input{Kind: MoveX, Value: 100})
	if !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("right gesture = %v", got)
	}
	if got := engine.Step(Input{Kind: ButtonRelease}); len(got) != 0 {
		t.Fatalf("gesture incorrectly emitted tap: %v", got)
	}
}

func TestRetriggerDistanceAndScroll(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	up := config.Action{Type: config.ActionNoctalia, Value: "volume up"}
	engine := New(100, 150, 0, Bindings{Right: right, ScrollUp: up})
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: 100}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("first step = %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 149}); len(got) != 0 {
		t.Fatalf("fired below second threshold: %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 1}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("second step = %v", got)
	}
	if got := engine.Step(Input{Kind: Scroll, Value: 1}); !reflect.DeepEqual(got, []config.Action{up}) {
		t.Fatalf("scroll = %v", got)
	}
}

func TestProgressTracksThresholdAndTriggerDirection(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	engine := New(100, 50, 0, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})

	engine.Step(Input{Kind: MoveX, Value: 25})
	if got := engine.Progress(); got.X != 0.25 || got.XDirection != "right" {
		t.Fatalf("initial X progress = %+v, want 0.25 right", got)
	}

	engine.Step(Input{Kind: MoveX, Value: 75})
	if got := engine.Progress(); got.X != 0 || got.TriggerCount != 1 ||
		got.TriggeredAxis != "x" || got.TriggeredDirection != "right" {
		t.Fatalf("progress after trigger = %+v, want reset progress and right trigger", got)
	}

	engine.Step(Input{Kind: MoveX, Value: -25})
	if got := engine.Progress(); got.X != 0.5 || got.XDirection != "left" {
		t.Fatalf("reversed X progress = %+v, want 0.5 left", got)
	}

	up := config.Action{Type: config.ActionNiri, Value: "up"}
	verticalEngine := New(100, 50, 0, Bindings{Up: up})
	verticalEngine.Step(Input{Kind: ButtonPress})
	verticalEngine.Step(Input{Kind: MoveY, Value: -25})
	if got := verticalEngine.Progress(); got.Y != 0.25 || got.YDirection != "up" {
		t.Fatalf("initial Y progress = %+v, want 0.25 up", got)
	}
}

func TestDistanceChangesApplyDuringActiveGesture(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "focus-column-right"}
	engine := New(100, 100, 0, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: 20}); len(got) != 0 {
		t.Fatalf("initial movement actions = %v", got)
	}

	engine.SetDistances(25, 30, 0)
	if got := engine.Step(Input{Kind: MoveX, Value: 5}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("gesture after dynamic distance update = %v, want %v", got, right)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 29}); len(got) != 0 {
		t.Fatalf("retrigger fired below dynamically updated distance: %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 1}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("retrigger after dynamic distance update = %v, want %v", got, right)
	}
}

func TestIndependentAxisGestureDistances(t *testing.T) {
	left := config.Action{Type: config.ActionNiri, Value: "focus-column-left"}
	down := config.Action{Type: config.ActionNiri, Value: "focus-workspace-down"}
	engine := NewWithAxes(
		config.GestureDistances{Threshold: 40, RetriggerDistance: 30, Escalation: 10},
		config.GestureDistances{Threshold: 100, RetriggerDistance: 70, Escalation: 20},
		true,
		Bindings{Left: left, Down: down},
	)
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: -40}); !reflect.DeepEqual(got, []config.Action{left}) {
		t.Fatalf("X gesture = %v, want %v", got, left)
	}
	if got := engine.Step(Input{Kind: MoveY, Value: 99}); len(got) != 0 {
		t.Fatalf("Y gesture fired below Y threshold = %v", got)
	}
	if got := engine.Step(Input{Kind: MoveY, Value: 1}); !reflect.DeepEqual(got, []config.Action{down}) {
		t.Fatalf("Y gesture = %v, want %v", got, down)
	}
	if got := engine.Step(Input{Kind: MoveY, Value: 89}); len(got) != 0 {
		t.Fatalf("Y retrigger fired below Y distance = %v", got)
	}
	if got := engine.Step(Input{Kind: MoveY, Value: 1}); !reflect.DeepEqual(got, []config.Action{down}) {
		t.Fatalf("Y retrigger = %v, want %v", got, down)
	}
}

func TestDistanceChangeCanCompleteAccumulatedMovement(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "focus-column-right"}
	engine := New(100, 100, 0, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: 50}); len(got) != 0 {
		t.Fatalf("initial movement actions = %v", got)
	}

	if got := engine.SetDistances(25, 30, 0); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("actions after threshold update = %v, want %v", got, right)
	}
}

func TestRetriggerEscalatesIndependently(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	engine := New(100, 100, 50, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	for i, distance := range []int32{100, 150, 200} {
		if got := engine.Step(Input{Kind: MoveX, Value: distance}); !reflect.DeepEqual(got, []config.Action{right}) {
			t.Fatalf("gesture %d after %d counts = %v, want right", i+1, distance, got)
		}
	}
}

func TestZeroEscalationUsesSameRetriggerDistance(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	engine := New(100, 100, 0, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	for i := 0; i < 3; i++ {
		if got := engine.Step(Input{Kind: MoveX, Value: 100}); !reflect.DeepEqual(got, []config.Action{right}) {
			t.Fatalf("gesture %d = %v, want right", i+1, got)
		}
	}
}

func TestDirectionChangeClearsMovementBuffer(t *testing.T) {
	left := config.Action{Type: config.ActionNiri, Value: "left"}
	up := config.Action{Type: config.ActionNiri, Value: "up"}
	tests := []struct {
		name     string
		first    Input
		reversed Input
		action   config.Action
	}{
		{
			name:     "horizontal",
			first:    Input{Kind: MoveX, Value: 80},
			reversed: Input{Kind: MoveX, Value: -100},
			action:   left,
		},
		{
			name:     "vertical",
			first:    Input{Kind: MoveY, Value: 80},
			reversed: Input{Kind: MoveY, Value: -100},
			action:   up,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := New(100, 100, 0, Bindings{Left: left, Up: up})
			engine.Step(Input{Kind: ButtonPress})
			if got := engine.Step(test.first); len(got) != 0 {
				t.Fatalf("initial movement actions = %v", got)
			}
			if got := engine.Step(test.reversed); !reflect.DeepEqual(got, []config.Action{test.action}) {
				t.Fatalf("reversed movement actions = %v, want %v", got, test.action)
			}
		})
	}
}

func TestMovementOutsideHoldDoesNothing(t *testing.T) {
	engine := New(100, 100, 0, Bindings{Right: config.Action{Type: config.ActionNiri, Value: "right"}})
	if got := engine.Step(Input{Kind: MoveX, Value: 500}); len(got) != 0 {
		t.Fatalf("actions outside hold = %v", got)
	}
}

func TestThumbwheelActionsRequireActiveHold(t *testing.T) {
	left := config.Action{Type: config.ActionNiri, Value: "left"}
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	tap := config.Action{Type: config.ActionNiri, Value: "toggle-overview"}
	engine := New(100, 100, 0, Bindings{Tap: tap, ThumbwheelLeft: left, ThumbwheelRight: right})
	if got := engine.Step(Input{Kind: ThumbwheelLeft}); len(got) != 0 {
		t.Fatalf("thumbwheel action outside hold = %v", got)
	}
	engine.Step(Input{Kind: ButtonPress})
	for _, test := range []struct {
		input Input
		want  config.Action
	}{
		{input: Input{Kind: ThumbwheelLeft}, want: left},
		{input: Input{Kind: ThumbwheelRight}, want: right},
	} {
		if got := engine.Step(test.input); !reflect.DeepEqual(got, []config.Action{test.want}) {
			t.Errorf("thumbwheel action = %v, want %v", got, test.want)
		}
	}
	if got := engine.Step(Input{Kind: ButtonRelease}); len(got) != 0 {
		t.Fatalf("thumbwheel actions incorrectly emitted tap: %v", got)
	}
}

func TestMarkUsedSuppressesTapForPassedThroughThumbwheel(t *testing.T) {
	tap := config.Action{Type: config.ActionNiri, Value: "toggle-overview"}
	engine := New(100, 100, 0, Bindings{Tap: tap})
	engine.Step(Input{Kind: ButtonPress})
	engine.MarkUsed()
	if got := engine.Step(Input{Kind: ButtonRelease}); len(got) != 0 {
		t.Fatalf("passed-through thumbwheel incorrectly emitted tap: %v", got)
	}
}
