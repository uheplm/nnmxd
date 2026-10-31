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
	Left, Right, Up, Down     string
	Tap, ScrollUp, ScrollDown string
}

type Engine struct {
	threshold, escalation int
	bindings              Bindings
	active, used          bool
	step, x, y            int
}

func New(threshold, escalation int, bindings Bindings) *Engine {
	return &Engine{threshold: threshold, escalation: escalation, bindings: bindings}
}

func (e *Engine) Active() bool { return e.active }

func (e *Engine) Step(input Input) []string {
	switch input.Kind {
	case ButtonPress:
		e.active, e.used = true, false
		e.step, e.x, e.y = 0, 0, 0
	case ButtonRelease:
		var actions []string
		if !e.used && e.bindings.Tap != "" {
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
		case input.Value > 0 && e.bindings.ScrollUp != "":
			e.used = true
			return []string{e.bindings.ScrollUp}
		case input.Value < 0 && e.bindings.ScrollDown != "":
			e.used = true
			return []string{e.bindings.ScrollDown}
		}
	}
	return nil
}

func (e *Engine) thresholdForStep() int {
	return e.threshold + e.step*e.escalation
}

func (e *Engine) horizontal() []string {
	threshold := e.thresholdForStep()
	switch {
	case e.x >= threshold && e.bindings.Right != "":
		return e.fire(e.bindings.Right)
	case e.x <= -threshold && e.bindings.Left != "":
		return e.fire(e.bindings.Left)
	}
	return nil
}

func (e *Engine) vertical() []string {
	threshold := e.thresholdForStep()
	switch {
	case e.y >= threshold && e.bindings.Down != "":
		return e.fire(e.bindings.Down)
	case e.y <= -threshold && e.bindings.Up != "":
		return e.fire(e.bindings.Up)
	}
	return nil
}

func (e *Engine) fire(action string) []string {
	e.used = true
	e.step++
	e.x, e.y = 0, 0
	return []string{action}
}
