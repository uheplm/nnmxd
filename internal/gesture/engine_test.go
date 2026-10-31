package gesture

import (
	"reflect"
	"testing"

	"github.com/uheplm/nnmxd/internal/config"
)

func TestTapAndGesture(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "focus-column-right"}
	tap := config.Action{Type: config.ActionNiri, Value: "toggle-overview"}
	engine := New(100, 250, Bindings{Right: right, Tap: tap})
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

func TestEscalationAndScroll(t *testing.T) {
	right := config.Action{Type: config.ActionNiri, Value: "right"}
	up := config.Action{Type: config.ActionNoctalia, Value: "volume up"}
	engine := New(100, 250, Bindings{Right: right, ScrollUp: up})
	engine.Step(Input{Kind: ButtonPress})
	if got := engine.Step(Input{Kind: MoveX, Value: 100}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("first step = %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 349}); len(got) != 0 {
		t.Fatalf("fired below second threshold: %v", got)
	}
	if got := engine.Step(Input{Kind: MoveX, Value: 1}); !reflect.DeepEqual(got, []config.Action{right}) {
		t.Fatalf("second step = %v", got)
	}
	if got := engine.Step(Input{Kind: Scroll, Value: 1}); !reflect.DeepEqual(got, []config.Action{up}) {
		t.Fatalf("scroll = %v", got)
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
			engine := New(100, 250, Bindings{Left: left, Up: up})
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
	engine := New(100, 250, Bindings{Right: config.Action{Type: config.ActionNiri, Value: "right"}})
	if got := engine.Step(Input{Kind: MoveX, Value: 500}); len(got) != 0 {
		t.Fatalf("actions outside hold = %v", got)
	}
}
