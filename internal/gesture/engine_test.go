package gesture

import (
	"reflect"
	"testing"
)

func TestTapAndGesture(t *testing.T) {
	right := Action{Type: "niri", Value: "focus-column-right"}
	tap := Action{Type: "niri", Value: "toggle-overview"}
	engine := New(100, 100, 0, Bindings{Right: right, Tap: tap})
	if got := engine.Step(Input{Kind: ButtonPress}); len(got) != 0 {
		t.Fatalf("press actions = %v", got)
	}
	if got := engine.Step(Input{Kind: ButtonRelease}); !reflect.DeepEqual(got, []Action{tap}) {
		t.Fatalf("tap = %v", got)
	}

	engine.Step(Input{Kind: ButtonPress})
	got := engine.Step(Input{Kind: MoveX, Value: 100})
	if !reflect.DeepEqual(got, []Action{right}) {
		t.Fatalf("right gesture = %v", got)
	}
	if got := engine.Step(Input{Kind: ButtonRelease}); len(got) != 0 {
		t.Fatalf("gesture incorrectly emitted tap: %v", got)
	}
}

func TestRetriggerDistanceAndScroll(t *testing.T) {
	right := Action{Type: "niri", Value: "right"}
	engine := New(100, 150, 0, Bindings{Right: right, ScrollUp: Action{Type: "noctalia", Value: "volume-up"}})
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: 100}); !reflect.DeepEqual(got, []Action{right}) {
		t.Fatalf("first step = %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 149}); len(got) != 0 {
		t.Fatalf("fired below second threshold: %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 1}); !reflect.DeepEqual(got, []Action{right}) {
		t.Fatalf("second step = %v", got)
	}
	if got := engine.Step(Input{Kind: Scroll, Value: 1}); !reflect.DeepEqual(got, []Action{{Type: "noctalia", Value: "volume-up"}}) {
		t.Fatalf("scroll = %v", got)
	}
}

func TestRetriggerEscalatesIndependently(t *testing.T) {
	right := Action{Type: "niri", Value: "right"}
	engine := New(100, 100, 50, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	for i, distance := range []int32{100, 150, 200} {
		if got := engine.Step(Input{Kind: MoveX, Value: distance}); !reflect.DeepEqual(got, []Action{right}) {
			t.Fatalf("gesture %d after %d counts = %v, want right", i+1, distance, got)
		}
	}
}

func TestZeroEscalationUsesSameRetriggerDistance(t *testing.T) {
	right := Action{Type: "niri", Value: "right"}
	engine := New(100, 100, 0, Bindings{Right: right})
	engine.Step(Input{Kind: ButtonPress})
	for i := 0; i < 3; i++ {
		if got := engine.Step(Input{Kind: MoveX, Value: 100}); !reflect.DeepEqual(got, []Action{right}) {
			t.Fatalf("gesture %d = %v, want right", i+1, got)
		}
	}
}

func TestMovementOutsideHoldDoesNothing(t *testing.T) {
	engine := New(100, 100, 0, Bindings{Right: Action{Type: "niri", Value: "right"}})
	if got := engine.Step(Input{Kind: MoveX, Value: 500}); len(got) != 0 {
		t.Fatalf("actions outside hold = %v", got)
	}
}
