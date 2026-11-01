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
