package gesture

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
	Left, Right, Up, Down     Action
	Tap, ScrollUp, ScrollDown Action
}

type Action struct {
	Type      string
	Value     string
	Arguments string
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

func (e *Engine) Step(input Input) []Action {
	switch input.Kind {
	case ButtonPress:
		e.active, e.used = true, false
		e.step, e.x, e.y = 0, 0, 0
	case ButtonRelease:
		var actions []Action
		if !e.used && e.bindings.Tap.Value != "" {
			actions = append(actions, e.bindings.Tap)
		}
		e.active, e.used = false, false
		e.step, e.x, e.y = 0, 0, 0
		return actions
	case MoveX:
		if e.active {
			e.x += int(input.Value)
			return e.horizontal()
		}
	case MoveY:
		if e.active {
			e.y += int(input.Value)
			return e.vertical()
		}
	case Scroll:
		if !e.active {
			return nil
		}
		switch {
		case input.Value > 0 && e.bindings.ScrollUp.Value != "":
			e.used = true
			return []Action{e.bindings.ScrollUp}
		case input.Value < 0 && e.bindings.ScrollDown.Value != "":
			e.used = true
			return []Action{e.bindings.ScrollDown}
		}
	}
	return nil
}

func (e *Engine) thresholdForStep() int {
	if e.step == 0 {
		return e.threshold
	}
	return e.retriggerDistance + e.step*e.escalation
}

func (e *Engine) horizontal() []Action {
	threshold := e.thresholdForStep()
	switch {
	case e.x >= threshold && e.bindings.Right.Value != "":
		return e.fire(e.bindings.Right)
	case e.x <= -threshold && e.bindings.Left.Value != "":
		return e.fire(e.bindings.Left)
	}
	return nil
}

func (e *Engine) vertical() []Action {
	threshold := e.thresholdForStep()
	switch {
	case e.y >= threshold && e.bindings.Down.Value != "":
		return e.fire(e.bindings.Down)
	case e.y <= -threshold && e.bindings.Up.Value != "":
		return e.fire(e.bindings.Up)
	}
	return nil
}

func (e *Engine) fire(action Action) []Action {
	e.used = true
	e.step++
	e.x, e.y = 0, 0
	return []Action{action}
}
