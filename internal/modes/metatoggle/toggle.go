// Package metatoggle implements the legacy "press thumb button to latch the
// Super key down" behaviour from mouse-meta v1. The physical mouse is read
// passively (no grab) so the compositor still receives all events; a minimal
// uinput device is created solely to inject the virtual Super keypress.
package metatoggle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/holoplot/go-evdev"

	"github.com/naudachu/mxmoused/internal/config"
)

// State is the Waybar-compatible JSON written to StateFile on each toggle.
type State struct {
	Text    string `json:"text"`
	Alt     string `json:"alt"`
	Class   string `json:"class"`
	Tooltip string `json:"tooltip"`
}

// BuildState returns the waybar state for the given toggle position.
func BuildState(on bool) State {
	if on {
		return State{Text: "META", Alt: "on", Class: "on", Tooltip: "Meta is ON"}
	}
	return State{Text: "", Alt: "off", Class: "off", Tooltip: "Meta is OFF"}
}

// Run is the meta-toggle mode entry point. Reads dev passively, creates a
// virtual keyboard with a single key, toggles that key on trigger button
// press, and (optionally) maintains a waybar state file.
func Run(ctx context.Context, dev *evdev.InputDevice, cfg config.MetaToggleConfig, triggerButton string) error {
	triggerCode, ok := evdev.KEYFromString[triggerButton]
	if !ok {
		return fmt.Errorf("unknown trigger button %q", triggerButton)
	}
	metaCode, ok := evdev.KEYFromString[cfg.Key]
	if !ok {
		return fmt.Errorf("unknown meta key %q", cfg.Key)
	}

	virt, err := evdev.CreateDevice(
		"mxmoused-meta",
		evdev.InputID{BusType: evdev.BUS_VIRTUAL, Vendor: 0x0001, Product: 0x0001, Version: 1},
		map[evdev.EvType][]evdev.EvCode{
			evdev.EV_KEY: {metaCode},
		},
	)
	if err != nil {
		return fmt.Errorf("create virtual keyboard: %w", err)
	}
	defer evdev.DestroyDevice(virt)

	if cfg.Waybar {
		if err := writeState(cfg.StateFile, BuildState(false)); err != nil {
			log.Printf("waybar state: %v", err)
		}
	}

	on := false
	events := make(chan *evdev.InputEvent, 64)
	errs := make(chan error, 1)
	go func() {
		for {
			ev, err := dev.ReadOne()
			if err != nil {
				errs <- err
				return
			}
			events <- ev
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if on {
				releaseMeta(virt, metaCode)
			}
			if cfg.Waybar {
				_ = writeState(cfg.StateFile, BuildState(false))
			}
			return nil

		case err := <-errs:
			return err

		case ev := <-events:
			if ev.Type != evdev.EV_KEY || ev.Code != triggerCode || ev.Value != 1 {
				continue
			}
			on = !on
			value := int32(0)
			if on {
				value = 1
			}
			if err := injectKey(virt, metaCode, value); err != nil {
				log.Printf("inject meta: %v", err)
			}
			if cfg.Waybar {
				if err := writeState(cfg.StateFile, BuildState(on)); err != nil {
					log.Printf("waybar state: %v", err)
				}
			}
		}
	}
}

func injectKey(virt *evdev.InputDevice, code evdev.EvCode, value int32) error {
	if err := virt.WriteOne(&evdev.InputEvent{Type: evdev.EV_KEY, Code: code, Value: value}); err != nil {
		return err
	}
	return virt.WriteOne(&evdev.InputEvent{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT})
}

func releaseMeta(virt *evdev.InputDevice, code evdev.EvCode) {
	_ = injectKey(virt, code, 0)
}

func writeState(path string, s State) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
