package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/holoplot/go-evdev"

	"github.com/uheplm/nnmxd/internal/config"
	"github.com/uheplm/nnmxd/internal/device"
	"github.com/uheplm/nnmxd/internal/gesture"
	"github.com/uheplm/nnmxd/internal/niri"
)

type Status struct {
	Running     bool   `json:"running"`
	Device      string `json:"device"`
	Mode        string `json:"mode"`
	AppID       string `json:"appId"`
	PresetID    string `json:"presetId"`
	PresetName  string `json:"presetName"`
	MetaOn      bool   `json:"metaOn"`
	LastError   string `json:"lastError"`
	UpdatedUnix int64  `json:"updatedUnix"`
}

type reporter struct {
	path string
	data Status
}

type waybarState struct {
	Text    string `json:"text"`
	Alt     string `json:"alt"`
	Class   string `json:"class"`
	Tooltip string `json:"tooltip"`
}

func (r *reporter) update() {
	r.data.UpdatedUnix = time.Now().Unix()
	data, err := json.Marshal(r.data)
	if err != nil {
		log.Printf("encode status: %v", err)
		return
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("create status directory: %v", err)
		return
	}
	temp, err := os.CreateTemp(dir, ".status-*")
	if err != nil {
		log.Printf("create status file: %v", err)
		return
	}
	tempPath := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		log.Printf("write status: %v", err)
		return
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		log.Printf("secure status file: %v", err)
		return
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		log.Printf("close status file: %v", err)
		return
	}
	if err := os.Rename(tempPath, r.path); err != nil {
		_ = os.Remove(tempPath)
		log.Printf("publish status: %v", err)
	}
}

func Run(ctx context.Context, configPath, statusPath string) error {
	status := &reporter{path: statusPath}
	defer func() {
		status.data = Status{}
		status.update()
	}()
	client := niri.New()
	for {
		if ctx.Err() != nil {
			return nil
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			status.data = Status{LastError: err.Error()}
			status.update()
			log.Printf("configuration: %v", err)
			wait(ctx, 3*time.Second)
			continue
		}
		writeMetaState(cfg, false)
		dev, err := device.Open(cfg.Device)
		if err != nil {
			status.data = Status{LastError: err.Error()}
			status.update()
			log.Printf("input device: %v", err)
			wait(ctx, 3*time.Second)
			continue
		}
		name, _ := dev.Name()
		status.data = Status{Running: true, Device: name}
		status.update()
		log.Printf("attached to %q (%s)", name, dev.Path())
		err = runDevice(ctx, dev, configPath, cfg, client, status)
		_ = dev.Close()
		if ctx.Err() != nil {
			continue
		}
		status.data = Status{LastError: err.Error()}
		status.update()
		log.Printf("input session ended: %v", err)
		wait(ctx, time.Second)
	}
}

func runDevice(
	ctx context.Context,
	dev *evdev.InputDevice,
	configPath string,
	initial config.Config,
	niriClient *niri.Client,
	status *reporter,
) error {
	triggerCode, ok := evdev.KEYFromString[initial.Device.TriggerButton]
	if !ok {
		return fmt.Errorf("unknown trigger button %q", initial.Device.TriggerButton)
	}
	if err := dev.Grab(); err != nil {
		return fmt.Errorf("grab device: %w", err)
	}
	defer func() { _ = dev.Ungrab() }()

	clone, err := evdev.CloneDevice("mxmouse-pointer", dev)
	if err != nil {
		return fmt.Errorf("create pointer clone (check /dev/uinput permissions): %w", err)
	}
	defer func() { _ = evdev.DestroyDevice(clone) }()

	keyboard, err := evdev.CreateDevice(
		"mxmouse-keys",
		evdev.InputID{BusType: evdev.BUS_VIRTUAL, Vendor: 0x0001, Product: 0x0001, Version: 1},
		map[evdev.EvType][]evdev.EvCode{evdev.EV_KEY: supportedMetaKeys()},
	)
	if err != nil {
		return fmt.Errorf("create virtual keyboard (check /dev/uinput permissions): %w", err)
	}
	defer func() { _ = evdev.DestroyDevice(keyboard) }()

	if err := dev.NonBlock(); err != nil {
		return fmt.Errorf("set input device nonblocking: %w", err)
	}
	events := make(chan *evdev.InputEvent, 128)
	readErrors := make(chan error, 1)
	go readEvents(ctx, dev, events, readErrors)

	currentConfig := initial
	var engine *gesture.Engine
	gestureActive := false
	pressedMeta := map[evdev.EvCode]bool{}
	for {
		select {
		case <-ctx.Done():
			releaseMetaKeys(keyboard, pressedMeta)
			writeMetaState(currentConfig, len(pressedMeta) > 0)
			return nil
		case err := <-readErrors:
			releaseMetaKeys(keyboard, pressedMeta)
			writeMetaState(currentConfig, len(pressedMeta) > 0)
			return err
		case event := <-events:
			if event == nil {
				continue
			}
			if event.Type == evdev.EV_KEY && event.Code == triggerCode {
				switch event.Value {
				case 1:
					loaded, loadErr := config.Load(configPath)
					if loadErr != nil {
						status.data.LastError = loadErr.Error()
						log.Printf("reload config: %v", loadErr)
					} else {
						currentConfig = loaded
						status.data.LastError = ""
					}
					if currentConfig.Device.TriggerButton != initial.Device.TriggerButton ||
						currentConfig.Device.Path != initial.Device.Path ||
						currentConfig.Device.AutoDetect != initial.Device.AutoDetect {
						status.data.LastError = "input-device settings changed; restart the user service to apply them"
					}
					focusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					appID, appErr := niriClient.FocusedAppID(focusCtx)
					cancel()
					if appErr != nil {
						status.data.LastError = appErr.Error()
						appID = ""
					}
					preset := currentConfig.PresetForApp(appID)
					status.data.AppID = appID
					status.data.PresetID = preset.ID
					status.data.PresetName = preset.Name
					status.data.Mode = string(preset.Mode)

					if preset.Mode == config.ModeMetaToggle {
						gestureActive = false
						key, exists := evdev.KEYFromString[preset.MetaKey]
						if !exists {
							status.data.LastError = fmt.Sprintf("unknown meta key %q", preset.MetaKey)
						} else if err := toggleKey(keyboard, key, pressedMeta); err != nil {
							status.data.LastError = err.Error()
						}
						forward(clone, event)
					} else {
						gestureActive = true
						engine = newEngine(preset)
						_ = engine.Step(gesture.Input{Kind: gesture.ButtonPress})
					}
					status.data.MetaOn = len(pressedMeta) > 0
					writeMetaState(currentConfig, status.data.MetaOn)
					status.update()
				case 0:
					if gestureActive && engine != nil {
						dispatchActions(niriClient, engine.Step(gesture.Input{Kind: gesture.ButtonRelease}))
						gestureActive = false
					} else {
						forward(clone, event)
					}
					status.update()
				}
				continue
			}

			if gestureActive && engine != nil && event.Type == evdev.EV_REL {
				switch event.Code {
				case evdev.REL_X:
					dispatchActions(niriClient, engine.Step(gesture.Input{Kind: gesture.MoveX, Value: event.Value}))
				case evdev.REL_Y:
					dispatchActions(niriClient, engine.Step(gesture.Input{Kind: gesture.MoveY, Value: event.Value}))
				case evdev.REL_WHEEL:
					dispatchActions(niriClient, engine.Step(gesture.Input{Kind: gesture.Scroll, Value: event.Value}))
				}
				continue
			}
			forward(clone, event)
		}
	}
}

func newEngine(preset config.Preset) *gesture.Engine {
	return gesture.New(preset.Threshold, preset.Escalation, gesture.Bindings{
		Left: preset.Bindings.Left, Right: preset.Bindings.Right,
		Up: preset.Bindings.Up, Down: preset.Bindings.Down,
		Tap: preset.Bindings.Tap, ScrollUp: preset.Bindings.ScrollUp,
		ScrollDown: preset.Bindings.ScrollDown,
	})
}

func supportedMetaKeys() []evdev.EvCode {
	names := []string{
		"KEY_LEFTMETA", "KEY_RIGHTMETA", "KEY_LEFTCTRL", "KEY_RIGHTCTRL",
		"KEY_LEFTALT", "KEY_RIGHTALT", "KEY_LEFTSHIFT", "KEY_RIGHTSHIFT",
	}
	keys := make([]evdev.EvCode, 0, len(names))
	for _, name := range names {
		if code, ok := evdev.KEYFromString[name]; ok {
			keys = append(keys, code)
		}
	}
	return keys
}

func toggleKey(keyboard *evdev.InputDevice, key evdev.EvCode, pressed map[evdev.EvCode]bool) error {
	value := int32(1)
	if pressed[key] {
		value = 0
	}
	if err := keyboard.WriteOne(&evdev.InputEvent{Type: evdev.EV_KEY, Code: key, Value: value}); err != nil {
		return fmt.Errorf("inject virtual key: %w", err)
	}
	if err := keyboard.WriteOne(&evdev.InputEvent{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT}); err != nil {
		return fmt.Errorf("sync virtual key: %w", err)
	}
	if value == 1 {
		pressed[key] = true
	} else {
		delete(pressed, key)
	}
	return nil
}

func releaseMetaKeys(keyboard *evdev.InputDevice, pressed map[evdev.EvCode]bool) {
	for key := range pressed {
		if err := toggleKey(keyboard, key, pressed); err != nil {
			log.Printf("release virtual key: %v", err)
		}
	}
}

func writeMetaState(cfg config.Config, on bool) {
	if !cfg.WriteWaybar {
		return
	}
	state := waybarState{}
	if on {
		state = waybarState{Text: "META", Alt: "on", Class: "on", Tooltip: "Meta is ON"}
	} else {
		state = waybarState{Alt: "off", Class: "off", Tooltip: "Meta is OFF"}
	}
	data, err := json.Marshal(state)
	if err != nil {
		log.Printf("encode Waybar state: %v", err)
		return
	}
	if err := os.WriteFile(cfg.MetaStateFile, append(data, '\n'), 0o644); err != nil {
		log.Printf("write Waybar state: %v", err)
	}
}

func dispatchActions(client *niri.Client, actions []string) {
	for _, action := range actions {
		client.Action(action)
	}
}

func forward(clone *evdev.InputDevice, event *evdev.InputEvent) {
	if err := clone.WriteOne(event); err != nil {
		log.Printf("forward input event: %v", err)
	}
}

func readEvents(
	ctx context.Context,
	dev *evdev.InputDevice,
	events chan<- *evdev.InputEvent,
	errs chan<- error,
) {
	for {
		if ctx.Err() != nil {
			return
		}
		event, err := dev.ReadOne()
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			select {
			case errs <- err:
			case <-ctx.Done():
			}
			return
		}
		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func StatusPath() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set")
	}
	return filepath.Join(runtimeDir, "mxmouse", "status.json"), nil
}

func ConfigPath() (string, error) {
	return config.DefaultConfigPath()
}

func WriteConfig(configPath, candidatePath string) error {
	if _, err := os.Stat(candidatePath); err != nil {
		return fmt.Errorf("read candidate config: %w", err)
	}
	cfg, err := config.Load(candidatePath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tempPath := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tempPath, configPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish config: %w", err)
	}
	return nil
}
