package gesture

import (
	"reflect"
	"testing"
)

func defaultBindings() Bindings {
	return Bindings{
		Left:       "focus-column-left",
		Right:      "focus-column-right",
		Up:         "focus-workspace-up",
		Down:       "focus-workspace-down",
		Tap:        "toggle-overview",
		ScrollUp:   "focus-workspace-up",
		ScrollDown: "focus-workspace-down",
	}
}

func feed(e *Engine, inputs ...Input) []string {
	var out []string
	for _, in := range inputs {
		out = append(out, e.Step(in)...)
	}
	return out
}

func TestTapFiresTapAction(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputButtonRelease},
	)
	want := []string{"toggle-overview"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if e.Active() {
		t.Error("engine should not be active after release")
	}
}

func TestDragRightFiresRight(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelX, Value: 50},
		Input{Kind: InputRelX, Value: 50}, // accum = 100, hits threshold
	)
	want := []string{"focus-column-right"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDragLeft(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelX, Value: -150},
	)
	want := []string{"focus-column-left"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDragUpDown(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	if got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelY, Value: 200},
	); !reflect.DeepEqual(got, []string{"focus-workspace-down"}) {
		t.Errorf("down: got %v", got)
	}
	// reset
	e = NewEngine(100, 250, defaultBindings())
	if got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelY, Value: -200},
	); !reflect.DeepEqual(got, []string{"focus-workspace-up"}) {
		t.Errorf("up: got %v", got)
	}
}

func TestEscalatingThreshold(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelX, Value: 100}, // step 0: threshold 100 → right
		Input{Kind: InputRelX, Value: 100}, // step 1: threshold 350 — no fire
		Input{Kind: InputRelX, Value: 250}, // total 350 → right
	)
	want := []string{"focus-column-right", "focus-column-right"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDragSuppressesTap(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelX, Value: 150}, // fires right, marks used
		Input{Kind: InputButtonRelease},    // used=true, no tap
	)
	want := []string{"focus-column-right"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScrollWhileHeld(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputScroll, Value: 1},
		Input{Kind: InputScroll, Value: -1},
		Input{Kind: InputButtonRelease},
	)
	want := []string{"focus-workspace-up", "focus-workspace-down"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMovementBeforePressIsIgnored(t *testing.T) {
	e := NewEngine(100, 250, defaultBindings())
	if got := feed(e,
		Input{Kind: InputRelX, Value: 500},
		Input{Kind: InputRelY, Value: -500},
		Input{Kind: InputScroll, Value: 1},
	); len(got) != 0 {
		t.Errorf("expected no actions before press, got %v", got)
	}
}

func TestEmptyBindingSkipsDirection(t *testing.T) {
	b := defaultBindings()
	b.Right = "" // disable
	e := NewEngine(100, 250, b)
	got := feed(e,
		Input{Kind: InputButtonPress},
		Input{Kind: InputRelX, Value: 500},
	)
	if len(got) != 0 {
		t.Errorf("empty binding should fire nothing, got %v", got)
	}
}
