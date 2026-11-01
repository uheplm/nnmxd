package gesture

import "github.com/uheplm/nnmxd/internal/config"

type InputKind uint8

const (
	ButtonPress InputKind = iota
	ButtonRelease
	MoveX
	MoveY
	Scroll
)

type Input struct {
	Kind  InputKind
	Value int32
}

type Bindings struct {
	Left, Right, Up, Down     config.Action
	Tap, ScrollUp, ScrollDown config.Action
}

type Engine struct {
	threshold, retriggerDistance, escalation int
	bindings                                 Bindings
	active, used                             bool
	step, x, y                               int
}

func New(threshold, retriggerDistance, escalation int, bindings Bindings) *Engine {
	return &Engine{
		threshold: threshold, retriggerDistance: retriggerDistance, escalation: escalation,
		bindings: bindings,
	}
}

func (e *Engine) Active() bool { return e.active }

func (e *Engine) Step(input Input) []config.Action {
	switch input.Kind {
	case ButtonPress:
		e.active, e.used = true, false
		e.step, e.x, e.y = 0, 0, 0
	case ButtonRelease:
		var actions []config.Action
		if !e.used && e.bindings.Tap.Value != "" {
			actions = append(actions, e.bindings.Tap)
		}
		e.active, e.used = false, false
		e.step, e.x, e.y = 0, 0, 0
		return actions
	case MoveX:
		if e.active {
			e.x = accumulate(e.x, input.Value)
			return e.horizontal()
		}
	case MoveY:
		if e.active {
			e.y = accumulate(e.y, input.Value)
			return e.vertical()
		}
	case Scroll:
		if !e.active {
			return nil
		}
		switch {
		case input.Value > 0 && e.bindings.ScrollUp.Value != "":
			e.used = true
			return []config.Action{e.bindings.ScrollUp}
		case input.Value < 0 && e.bindings.ScrollDown.Value != "":
			e.used = true
			return []config.Action{e.bindings.ScrollDown}
		}
	}
	return nil
}

func accumulate(current int, delta int32) int {
	if current != 0 && delta != 0 && (current < 0) != (delta < 0) {
		current = 0
	}
	return current + int(delta)
}

func (e *Engine) thresholdForStep() int {
	if e.step == 0 {
		return e.threshold
	}
	return e.retriggerDistance + e.step*e.escalation
}

func (e *Engine) horizontal() []config.Action {
	threshold := e.thresholdForStep()
	switch {
	case e.x >= threshold && e.bindings.Right.Value != "":
		return e.fire(e.bindings.Right)
	case e.x <= -threshold && e.bindings.Left.Value != "":
		return e.fire(e.bindings.Left)
	}
	return nil
}

func (e *Engine) vertical() []config.Action {
	threshold := e.thresholdForStep()
	switch {
	case e.y >= threshold && e.bindings.Down.Value != "":
		return e.fire(e.bindings.Down)
	case e.y <= -threshold && e.bindings.Up.Value != "":
		return e.fire(e.bindings.Up)
	}
	return nil
}

func (e *Engine) fire(action config.Action) []config.Action {
	e.used = true
	e.step++
	e.x, e.y = 0, 0
	return []config.Action{action}
}
