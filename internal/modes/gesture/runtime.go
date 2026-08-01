package gesture

import (
	"context"
	"fmt"
	"log"

	"github.com/holoplot/go-evdev"

	"github.com/naudachu/mxmoused/internal/config"
)

// Dispatcher fires a named action. The gesture mode calls Dispatch for
// every action produced by the Engine. Implementations decide whether the
// action goes to niri, logid, or elsewhere — see internal/action.
type Dispatcher interface {
	Dispatch(name string)
}

// Run is the gesture mode entry point. It grabs dev exclusively, clones it as
// a virtual uinput device, and pumps events through the Engine until ctx is
// cancelled. On exit the real device is ungrabbed and the clone destroyed.
func Run(ctx context.Context, dev *evdev.InputDevice, d Dispatcher, cfg config.GestureConfig, triggerButton string) error {
	triggerCode, ok := evdev.KEYFromString[triggerButton]
	if !ok {
		return fmt.Errorf("unknown trigger button %q", triggerButton)
	}

	if err := dev.Grab(); err != nil {
		return fmt.Errorf("grab device: %w", err)
	}
	defer dev.Ungrab()

	clone, err := evdev.CloneDevice("mxmoused", dev)
	if err != nil {
		return fmt.Errorf("clone device: %w", err)
	}
	defer evdev.DestroyDevice(clone)

	engine := NewEngine(cfg.Threshold, cfg.Escalation, Bindings{
		Left:       cfg.Bindings.Left,
		Right:      cfg.Bindings.Right,
		Up:         cfg.Bindings.Up,
		Down:       cfg.Bindings.Down,
		Tap:        cfg.Bindings.Tap,
		ScrollUp:   cfg.Bindings.ScrollUp,
		ScrollDown: cfg.Bindings.ScrollDown,
	})

	events := make(chan *evdev.InputEvent, 128)
	errs := make(chan error, 1)
	go pumpEvents(dev, events, errs)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			return fmt.Errorf("read: %w", err)
		case ev := <-events:
			handleEvent(ev, engine, clone, d, triggerCode)
		}
	}
}

func pumpEvents(dev *evdev.InputDevice, out chan<- *evdev.InputEvent, errs chan<- error) {
	for {
		ev, err := dev.ReadOne()
		if err != nil {
			errs <- err
			return
		}
		out <- ev
	}
}

func handleEvent(ev *evdev.InputEvent, e *Engine, clone *evdev.InputDevice, d Dispatcher, triggerCode evdev.EvCode) {
	// Trigger button is always intercepted and never forwarded.
	if ev.Type == evdev.EV_KEY && ev.Code == triggerCode {
		switch ev.Value {
		case 1:
			fire(d, e.Step(Input{Kind: InputButtonPress}))
		case 0:
			fire(d, e.Step(Input{Kind: InputButtonRelease}))
		}
		return
	}

	// While the button is held, consume all EV_REL events instead of
	// forwarding them. This suppresses cursor drift during a gesture.
	if e.Active() && ev.Type == evdev.EV_REL {
		switch ev.Code {
		case evdev.REL_X:
			fire(d, e.Step(Input{Kind: InputRelX, Value: ev.Value}))
		case evdev.REL_Y:
			fire(d, e.Step(Input{Kind: InputRelY, Value: ev.Value}))
		case evdev.REL_WHEEL:
			fire(d, e.Step(Input{Kind: InputScroll, Value: ev.Value}))
		}
		return
	}

	// Forward everything else (clicks, scroll outside gesture, SYN, MSC, ...).
	if err := clone.WriteOne(ev); err != nil {
		log.Printf("forward: %v", err)
	}
}

func fire(d Dispatcher, actions []string) {
	for _, a := range actions {
		d.Dispatch(a)
	}
}
