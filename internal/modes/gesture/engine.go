// Package gesture implements macOS-style gesture navigation: hold a trigger
// button and drag (or scroll) the mouse to fire compositor actions.
//
// The Engine in this file is a pure state machine. It consumes abstract
// Inputs and returns action names. It has no dependency on evdev, uinput, or
// any external process — this makes it straightforward to unit-test and
// lets the runtime.go file remain a thin adapter.
package gesture

// InputKind identifies the kind of abstract event fed to the Engine.
type InputKind uint8

const (
	InputButtonPress InputKind = iota
	InputButtonRelease
	InputRelX
	InputRelY
	// InputScroll carries a signed scroll delta (positive = up, negative = down).
	InputScroll
)

// Input is the abstract event type consumed by the Engine.
type Input struct {
	Kind  InputKind
	Value int32
}

// Bindings maps gesture directions and tap to compositor action names. Empty
// strings disable that direction.
type Bindings struct {
	Left       string
	Right      string
	Up         string
	Down       string
	Tap        string
	ScrollUp   string
	ScrollDown string
}

// Engine holds gesture recognition state for a single trigger button.
type Engine struct {
	threshold  int
	escalation int
	bindings   Bindings

	active bool
	used   bool
	step   int
	accumX int
	accumY int
}

// NewEngine constructs an Engine. threshold is the pixel distance required to
// fire the first gesture step; each subsequent step within the same hold
// requires an additional `escalation` pixels.
func NewEngine(threshold, escalation int, b Bindings) *Engine {
	return &Engine{threshold: threshold, escalation: escalation, bindings: b}
}

// Active reports whether the trigger button is currently held. Callers use
// this to decide whether to forward REL events to the uinput clone.
func (e *Engine) Active() bool { return e.active }

// Step consumes one Input and returns zero or more action names to fire.
func (e *Engine) Step(in Input) []string {
	switch in.Kind {
	case InputButtonPress:
		e.active = true
		e.used = false
		e.step = 0
		e.accumX = 0
		e.accumY = 0
		return nil

	case InputButtonRelease:
		var actions []string
		if !e.used && e.bindings.Tap != "" {
			actions = []string{e.bindings.Tap}
		}
		e.active = false
		e.used = false
		e.step = 0
		e.accumX = 0
		e.accumY = 0
		return actions

	case InputRelX:
		if !e.active {
			return nil
		}
		e.accumX += int(in.Value)
		return e.checkHorizontal()

	case InputRelY:
		if !e.active {
			return nil
		}
		e.accumY += int(in.Value)
		return e.checkVertical()

	case InputScroll:
		if !e.active {
			return nil
		}
		switch {
		case in.Value > 0 && e.bindings.ScrollUp != "":
			e.used = true
			return []string{e.bindings.ScrollUp}
		case in.Value < 0 && e.bindings.ScrollDown != "":
			e.used = true
			return []string{e.bindings.ScrollDown}
		}
	}
	return nil
}

func (e *Engine) stepThreshold() int {
	return e.threshold + e.step*e.escalation
}

func (e *Engine) checkHorizontal() []string {
	t := e.stepThreshold()
	switch {
	case e.accumX >= t && e.bindings.Right != "":
		e.used = true
		e.step++
		e.accumX, e.accumY = 0, 0
		return []string{e.bindings.Right}
	case e.accumX <= -t && e.bindings.Left != "":
		e.used = true
		e.step++
		e.accumX, e.accumY = 0, 0
		return []string{e.bindings.Left}
	}
	return nil
}

func (e *Engine) checkVertical() []string {
	t := e.stepThreshold()
	switch {
	case e.accumY >= t && e.bindings.Down != "":
		e.used = true
		e.step++
		e.accumX, e.accumY = 0, 0
		return []string{e.bindings.Down}
	case e.accumY <= -t && e.bindings.Up != "":
		e.used = true
		e.step++
		e.accumX, e.accumY = 0, 0
		return []string{e.bindings.Up}
	}
	return nil
}
