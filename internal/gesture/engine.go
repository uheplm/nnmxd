package gesture

import (
	"math"

	"github.com/uheplm/nnmxd/internal/config"
)

type InputKind uint8

const (
	ButtonPress InputKind = iota
	ButtonRelease
	MoveX
	MoveY
	Scroll
	ThumbwheelLeft
	ThumbwheelRight
)

type Input struct {
	Kind  InputKind
	Value int32
}

type Bindings struct {
	Left, Right, Up, Down     config.Action
	Tap, ScrollUp, ScrollDown config.Action
	ThumbwheelLeft            config.Action
	ThumbwheelRight           config.Action
}

type Engine struct {
	xAxis, yAxis             config.GestureDistances
	bindings                 Bindings
	separateAxisSettings     bool
	active, used             bool
	step, xStep, yStep, x, y int
	triggerCount             uint64
	triggeredAxis            string
	triggeredDirection       string
}

type Progress struct {
	X                  float64
	XDirection         string
	Y                  float64
	YDirection         string
	TriggerCount       uint64
	TriggeredAxis      string
	TriggeredDirection string
}

func New(threshold, retriggerDistance, escalation int, bindings Bindings) *Engine {
	distances := config.GestureDistances{
		Threshold: threshold, RetriggerDistance: retriggerDistance, Escalation: escalation,
	}
	return NewWithAxes(distances, distances, false, bindings)
}

func NewWithAxes(
	xAxis, yAxis config.GestureDistances,
	separateAxisSettings bool,
	bindings Bindings,
) *Engine {
	return &Engine{
		xAxis: xAxis, yAxis: yAxis,
		separateAxisSettings: separateAxisSettings, bindings: bindings,
	}
}

func (e *Engine) Active() bool { return e.active }

func (e *Engine) Progress() Progress {
	progress := Progress{
		TriggerCount:       e.triggerCount,
		TriggeredAxis:      e.triggeredAxis,
		TriggeredDirection: e.triggeredDirection,
	}
	if !e.active {
		return progress
	}
	progress.X, progress.XDirection = axisProgress(
		e.x, e.thresholdForStep(e.xAxis, e.xStep), "right", "left",
	)
	progress.Y, progress.YDirection = axisProgress(
		e.y, e.thresholdForStep(e.yAxis, e.yStep), "down", "up",
	)
	return progress
}

func (e *Engine) MarkUsed() {
	if e.active {
		e.used = true
	}
}

func (e *Engine) SetDistances(threshold, retriggerDistance, escalation int) []config.Action {
	distances := config.GestureDistances{
		Threshold: threshold, RetriggerDistance: retriggerDistance, Escalation: escalation,
	}
	return e.SetAxisDistances(distances, distances)
}

func (e *Engine) SetAxisDistances(xAxis, yAxis config.GestureDistances) []config.Action {
	e.xAxis, e.yAxis = xAxis, yAxis
	if !e.active {
		return nil
	}
	if actions := e.horizontal(); len(actions) > 0 {
		return actions
	}
	return e.vertical()
}

func (e *Engine) Step(input Input) []config.Action {
	switch input.Kind {
	case ButtonPress:
		e.active, e.used = true, false
		e.step, e.xStep, e.yStep, e.x, e.y = 0, 0, 0, 0, 0
		e.triggerCount = 0
		e.triggeredAxis, e.triggeredDirection = "", ""
	case ButtonRelease:
		var actions []config.Action
		if !e.used && e.bindings.Tap.Value != "" {
			actions = append(actions, e.bindings.Tap)
		}
		e.active, e.used = false, false
		e.step, e.xStep, e.yStep, e.x, e.y = 0, 0, 0, 0, 0
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
	case ThumbwheelLeft:
		if e.active && e.bindings.ThumbwheelLeft.Value != "" {
			e.used = true
			return []config.Action{e.bindings.ThumbwheelLeft}
		}
	case ThumbwheelRight:
		if e.active && e.bindings.ThumbwheelRight.Value != "" {
			e.used = true
			return []config.Action{e.bindings.ThumbwheelRight}
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

func (e *Engine) thresholdForStep(distances config.GestureDistances, axisStep int) int {
	step := e.step
	if e.separateAxisSettings {
		step = axisStep
	}
	if step == 0 {
		return distances.Threshold
	}
	return distances.RetriggerDistance + step*distances.Escalation
}

func (e *Engine) horizontal() []config.Action {
	threshold := e.thresholdForStep(e.xAxis, e.xStep)
	switch {
	case e.x >= threshold && e.bindings.Right.Value != "":
		return e.fire(e.bindings.Right, "x", "right")
	case e.x <= -threshold && e.bindings.Left.Value != "":
		return e.fire(e.bindings.Left, "x", "left")
	}
	return nil
}

func (e *Engine) vertical() []config.Action {
	threshold := e.thresholdForStep(e.yAxis, e.yStep)
	switch {
	case e.y >= threshold && e.bindings.Down.Value != "":
		return e.fire(e.bindings.Down, "y", "down")
	case e.y <= -threshold && e.bindings.Up.Value != "":
		return e.fire(e.bindings.Up, "y", "up")
	}
	return nil
}

func axisProgress(value, threshold int, positiveDirection, negativeDirection string) (float64, string) {
	if value == 0 || threshold <= 0 {
		return 0, ""
	}
	direction := positiveDirection
	if value < 0 {
		direction = negativeDirection
	}
	return math.Min(1, math.Abs(float64(value))/float64(threshold)), direction
}

func (e *Engine) fire(action config.Action, axis, direction string) []config.Action {
	e.used = true
	e.step++
	e.triggerCount++
	e.triggeredAxis = axis
	e.triggeredDirection = direction
	if axis == "x" {
		e.xStep++
	} else {
		e.yStep++
	}
	e.x, e.y = 0, 0
	return []config.Action{action}
}
