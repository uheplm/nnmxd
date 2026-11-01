package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/holoplot/go-evdev"

	"github.com/uheplm/nnmxd/internal/config"
	"github.com/uheplm/nnmxd/internal/device"
	"github.com/uheplm/nnmxd/internal/gesture"
	"github.com/uheplm/nnmxd/internal/hidpp"
	"github.com/uheplm/nnmxd/internal/niri"
)

type Status struct {
	Running           bool   `json:"running"`
	Device            string `json:"device"`
	Mode              string `json:"mode"`
	AppID             string `json:"appId"`
	PresetID          string `json:"presetId"`
	PresetName        string `json:"presetName"`
	MetaOn            bool   `json:"metaOn"`
	LastError         string `json:"lastError"`
	AppliedGeneration int    `json:"appliedGeneration"`
	UpdatedUnix       int64  `json:"updatedUnix"`
}

type reporter struct {
	path              string
	data              Status
	appliedGeneration int
}

type thumbwheelRemainderKey struct {
	code     evdev.EvCode
	presetID string
	held     bool
}

type dpiRequest struct {
	presetID        string
	value           int
	captureBaseline bool
}

type dpiResult struct {
	request dpiRequest
	err     error
}

func (r *reporter) update() {
	r.data.AppliedGeneration = r.appliedGeneration
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
	status := &reporter{
		path:              statusPath,
		appliedGeneration: previousAppliedGeneration(configPath, statusPath),
	}
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
	dpiRequests := make(chan dpiRequest, 1)
	dpiResults := make(chan dpiResult)
	go runDPIWorker(ctx, dev.Path(), dpiRequests, dpiResults)
	var engine *gesture.Engine
	gestureActive := false
	thumbButtonHeld := false
	var heldPreset config.Preset
	focusedAppID := status.data.AppID
	focusUpdates := make(chan string, 1)
	go pollFocusedAppID(ctx, niriClient, focusUpdates)
	lastDPIRequest := dpiRequest{}
	queueProfileDPI := func(appID string) {
		preset := currentConfig.PresetForApp(appID)
		request := dpiRequest{
			presetID: preset.ID, value: effectiveDPI(preset, currentConfig.Device),
			captureBaseline: needsBaselineDPI(currentConfig),
		}
		if request == lastDPIRequest {
			return
		}
		lastDPIRequest = request
		select {
		case dpiRequests <- request:
		default:
			select {
			case <-dpiRequests:
			default:
			}
			select {
			case dpiRequests <- request:
			default:
			}
		}
	}
	queueProfileDPI(focusedAppID)
	pressedMeta := map[evdev.EvCode]bool{}
	consumedSideButtons := map[evdev.EvCode]bool{}
	consumedThumbClicks := map[evdev.EvCode]bool{}
	wheelRemainders := map[evdev.EvCode]float64{}
	thumbwheelRemainders := map[thumbwheelRemainderKey]int64{}
	relativeCodes := dev.CapableEvents(evdev.EV_REL)
	hasHighResolutionHorizontalWheel := false
	for _, code := range relativeCodes {
		if code == evdev.REL_HWHEEL_HI_RES {
			hasHighResolutionHorizontalWheel = true
			break
		}
	}
	suppressTriggerRelease := false
	for {
		select {
		case <-ctx.Done():
			releaseMetaKeys(keyboard, pressedMeta)
			return nil
		case appID := <-focusUpdates:
			if appID != focusedAppID {
				focusedAppID = appID
				preset := currentConfig.PresetForApp(appID)
				status.data.AppID = appID
				status.data.PresetID = preset.ID
				status.data.PresetName = preset.Name
				status.data.Mode = string(preset.Mode)
				queueProfileDPI(appID)
				status.update()
			}
		case result := <-dpiResults:
			if result.request == lastDPIRequest {
				if result.err != nil {
					status.data.LastError = result.err.Error()
				} else if strings.Contains(status.data.LastError, "DPI") ||
					strings.Contains(status.data.LastError, "dpi") {
					status.data.LastError = ""
				}
				status.update()
			}
		case err := <-readErrors:
			releaseMetaKeys(keyboard, pressedMeta)
			return err
		case event := <-events:
			if event == nil {
				continue
			}
			if event.Type == evdev.EV_KEY && event.Code == triggerCode {
				switch event.Value {
				case 1:
					thumbButtonHeld = true
					suppressTriggerRelease = false
					previousGeneration := status.appliedGeneration
					loaded, loadErr := config.Load(configPath)
					if loadErr != nil {
						status.data.LastError = loadErr.Error()
						log.Printf("reload config: %v", loadErr)
					} else {
						currentConfig = loaded
						status.data.LastError = ""
					}
					suppressTriggerRelease = loadErr == nil &&
						shouldSuppressTrigger(loaded.ApplyGeneration, status.appliedGeneration)
					log.Printf(
						"thumb-button press: config generation %d (applied %d), device DPI request %d",
						currentConfig.ApplyGeneration,
						previousGeneration,
						currentConfig.Device.DPI,
					)
					if suppressTriggerRelease {
						log.Printf(
							"consuming thumb-button press for config generation %d; assigned action will be skipped",
							loaded.ApplyGeneration,
						)
						if err := storeAppliedGeneration(configPath, loaded.ApplyGeneration); err != nil {
							status.data.LastError = fmt.Sprintf("record applied config generation: %v", err)
							log.Printf("record applied config generation: %v", err)
						} else {
							status.appliedGeneration = loaded.ApplyGeneration
						}
					}
					queueProfileDPI(focusedAppID)
					if currentConfig.Device.TriggerButton != initial.Device.TriggerButton ||
						currentConfig.Device.Path != initial.Device.Path ||
						currentConfig.Device.AutoDetect != initial.Device.AutoDetect {
						status.data.LastError = "input-device settings changed; restart the user service to apply them"
					}
					if !suppressTriggerRelease {
						focusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
						appID, appErr := niriClient.FocusedAppID(focusCtx)
						cancel()
						if appErr != nil {
							status.data.LastError = appErr.Error()
							appID = focusedAppID
						} else {
							focusedAppID = appID
						}
						preset := currentConfig.PresetForApp(appID)
						queueProfileDPI(appID)
						heldPreset = preset
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
					}
					status.data.MetaOn = len(pressedMeta) > 0
					status.update()
				case 0:
					thumbButtonHeld = false
					if suppressTriggerRelease {
						log.Printf("thumb-button release consumed after config apply; assigned action was skipped")
						suppressTriggerRelease = false
					} else if gestureActive && engine != nil {
						dispatchActions(niriClient, engine.Step(gesture.Input{Kind: gesture.ButtonRelease}))
						gestureActive = false
					} else {
						forward(clone, event)
					}
					status.update()
				}
				continue
			}

			if handleThumbClickEvent(
				heldPreset,
				event,
				thumbButtonHeld && !suppressTriggerRelease && gestureActive,
				consumedThumbClicks,
				func(action config.Action) {
					if engine != nil {
						engine.MarkUsed()
					}
					dispatchActions(niriClient, []config.Action{action})
				},
			) {
				continue
			}

			if handleSideButtonEvent(
				currentConfig.PresetForApp(focusedAppID),
				event,
				consumedSideButtons,
				func(action config.Action) {
					dispatchActions(niriClient, []config.Action{action})
				},
			) {
				continue
			}

			if event.Type == evdev.EV_REL {
				if hasHighResolutionHorizontalWheel && isLegacyHighResolutionWheel(event.Code) {
					continue
				}
				if direction, isHorizontalWheel := horizontalWheelDirection(event.Code, event.Value); isHorizontalWheel {
					buttonPressed := thumbButtonHeld && !suppressTriggerRelease
					if buttonPressed && gestureActive && engine != nil {
						engine.MarkUsed()
					}
					if !thumbButtonHeld || !suppressTriggerRelease {
						var preset config.Preset
						if buttonPressed {
							preset = heldPreset
						} else {
							preset = currentConfig.PresetForApp(focusedAppID)
						}
						action := thumbwheelAction(preset, direction, buttonPressed)
						if action.Value != "" {
							remainderKey := thumbwheelRemainderKey{
								code: event.Code, presetID: preset.ID, held: buttonPressed,
							}
							count, actionDirection := thumbwheelActionCount(
								event.Code, event.Value, thumbwheelRemainders, remainderKey)
							if count > 0 {
								var actions []config.Action
								for i := 0; i < count; i++ {
									if buttonPressed && gestureActive && engine != nil {
										actions = append(actions, engine.Step(gesture.Input{Kind: actionDirection})...)
									} else {
										actions = append(actions, action)
									}
								}
								dispatchActions(niriClient, actions)
							}
							continue
						}
					}
					preset := currentConfig.PresetForApp(focusedAppID)
					if buttonPressed {
						preset = heldPreset
					}
					if percent, ok := wheelScale(event.Code, effectiveScrollSettings(preset, currentConfig.Device)); ok {
						scaled := *event
						scaled.Value = scaleWheelValue(event.Code, event.Value, percent, wheelRemainders)
						if scaled.Value != 0 {
							forward(clone, &scaled)
						}
					}
					continue
				}
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
			if event.Type == evdev.EV_REL {
				preset := currentConfig.PresetForApp(focusedAppID)
				if thumbButtonHeld && !suppressTriggerRelease {
					preset = heldPreset
				}
				if percent, ok := wheelScale(event.Code, effectiveScrollSettings(preset, currentConfig.Device)); ok {
					scaled := *event
					scaled.Value = scaleWheelValue(event.Code, event.Value, percent, wheelRemainders)
					if scaled.Value != 0 {
						forward(clone, &scaled)
					}
					continue
				}
			}
			forward(clone, event)
		}
	}
}

func shouldSuppressTrigger(generation, appliedGeneration int) bool {
	return generation > appliedGeneration
}

func retryDPIApply(apply func() error, attempts int, delay time.Duration) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = apply()
		if err == nil {
			return nil
		}
		if attempt < attempts {
			log.Printf("retrying DPI apply during config confirmation (%d/%d): %v", attempt+1, attempts, err)
			time.Sleep(delay)
		}
	}
	return err
}

func previousAppliedGeneration(configPath, statusPath string) int {
	state, err := os.ReadFile(configPath + ".applied-generation")
	if err == nil {
		generation, parseErr := strconv.Atoi(strings.TrimSpace(string(state)))
		if parseErr != nil || generation < 0 {
			log.Printf("parse applied config generation: invalid state file contents")
		} else {
			return generation
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("read applied config generation: %v", err)
	}

	data, err := os.ReadFile(statusPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		log.Printf("read previous service status: %v", err)
		return 0
	}
	var previous Status
	if err := json.Unmarshal(data, &previous); err != nil {
		log.Printf("parse previous service status: %v", err)
		return 0
	}
	if previous.AppliedGeneration < 0 {
		log.Printf("previous service status has a negative applied generation")
		return 0
	}
	return previous.AppliedGeneration
}

func storeAppliedGeneration(configPath string, generation int) error {
	path := configPath + ".applied-generation"
	temp, err := os.CreateTemp(filepath.Dir(path), ".applied-generation-*")
	if err != nil {
		return fmt.Errorf("create applied-generation state: %w", err)
	}
	tempPath := temp.Name()
	if _, err := fmt.Fprintln(temp, generation); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write applied-generation state: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("secure applied-generation state: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close applied-generation state: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish applied-generation state: %w", err)
	}
	return nil
}

func wheelScale(code evdev.EvCode, settings config.Device) (int, bool) {
	switch code {
	case evdev.EvCode(0x06), evdev.EvCode(0x0c):
		return settings.HorizontalScroll, true
	case evdev.EvCode(0x08), evdev.EvCode(0x0b):
		return settings.VerticalScroll, true
	default:
		return 0, false
	}
}

func effectiveDPI(preset config.Preset, device config.Device) int {
	if preset.DPI != 0 {
		return preset.DPI
	}
	return device.DPI
}

func needsBaselineDPI(cfg config.Config) bool {
	if cfg.Device.DPI != 0 {
		return false
	}
	for _, preset := range cfg.Presets {
		if preset.DPI != 0 {
			return true
		}
	}
	return false
}

func effectiveScrollSettings(preset config.Preset, device config.Device) config.Device {
	if preset.VerticalScroll != 0 {
		device.VerticalScroll = preset.VerticalScroll
	}
	if preset.HorizontalScroll != 0 {
		device.HorizontalScroll = preset.HorizontalScroll
	}
	return device
}

func runDPIWorker(
	ctx context.Context,
	inputPath string,
	requests <-chan dpiRequest,
	results chan<- dpiResult,
) {
	baselineDPI := 0
	baselineKnown := false
	currentDPI := baselineDPI

	for {
		var request dpiRequest
		select {
		case <-ctx.Done():
			return
		case request = <-requests:
		}

		for attempt := 0; ; attempt++ {
			var err error
			if request.captureBaseline && !baselineKnown {
				baselineDPI, err = hidpp.CurrentDPI(inputPath)
				if err != nil {
					err = fmt.Errorf("read baseline mouse DPI for profile switching: %w", err)
					log.Printf("%v", err)
				} else {
					baselineKnown = true
					currentDPI = baselineDPI
					log.Printf("captured baseline mouse DPI %d for profile switching", baselineDPI)
				}
			}
			targetDPI := request.value
			if targetDPI == 0 && baselineKnown {
				targetDPI = baselineDPI
			}
			switch {
			case err != nil:
			case targetDPI == currentDPI:
			case targetDPI == 0:
			default:
				log.Printf("setting DPI for profile %q to %d (requested profile DPI: %d)", request.presetID, targetDPI, request.value)
				err = hidpp.SetDPI(inputPath, targetDPI)
				if err == nil {
					currentDPI = targetDPI
					log.Printf("set mouse DPI to %d for profile %q", targetDPI, request.presetID)
				} else {
					log.Printf("set DPI for profile %q to %d failed: %v", request.presetID, targetDPI, err)
				}
			}
			select {
			case results <- dpiResult{request: request, err: err}:
			case <-ctx.Done():
				return
			}
			if err == nil {
				break
			}
			if !retryableDPIError(err) {
				break
			}

			backoff := dpiRetryDelay(attempt)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case request = <-requests:
				timer.Stop()
				attempt = -1
			case <-timer.C:
			}
		}
	}
}

func dpiRetryDelay(attempt int) time.Duration {
	switch {
	case attempt < 1:
		return time.Second
	case attempt < 3:
		return 5 * time.Second
	case attempt < 6:
		return 15 * time.Second
	default:
		return time.Minute
	}
}

func retryableDPIError(err error) bool {
	message := strings.ToLower(err.Error())
	return !strings.Contains(message, "not supported") &&
		!strings.Contains(message, "outside the supported range")
}

func scaleWheelValue(code evdev.EvCode, value int32, percent int, remainders map[evdev.EvCode]float64) int32 {
	if percent == 100 {
		remainders[code] = 0
		return value
	}
	scaled := float64(value)*float64(percent)/100 + remainders[code]
	result := int32(scaled)
	remainders[code] = scaled - float64(result)
	return result
}

func newEngine(preset config.Preset) *gesture.Engine {
	return gesture.New(preset.Threshold, preset.RetriggerDistance, preset.Escalation, gesture.Bindings{
		Left: preset.Bindings.Left, Right: preset.Bindings.Right,
		Up: preset.Bindings.Up, Down: preset.Bindings.Down,
		Tap: preset.Bindings.Tap, ScrollUp: preset.Bindings.ScrollUp,
		ScrollDown:      preset.Bindings.ScrollDown,
		ThumbwheelLeft:  preset.Bindings.ThumbwheelButtonLeft,
		ThumbwheelRight: preset.Bindings.ThumbwheelButtonRight,
	})
}

func horizontalWheelDirection(code evdev.EvCode, value int32) (gesture.InputKind, bool) {
	if code != evdev.REL_HWHEEL && code != evdev.REL_HWHEEL_HI_RES || value == 0 {
		return 0, false
	}
	if value < 0 {
		return gesture.ThumbwheelLeft, true
	}
	return gesture.ThumbwheelRight, true
}

func isLegacyHighResolutionWheel(code evdev.EvCode) bool {
	return code == evdev.REL_HWHEEL
}

func thumbwheelAction(preset config.Preset, direction gesture.InputKind, buttonPressed bool) config.Action {
	if buttonPressed {
		switch direction {
		case gesture.ThumbwheelLeft:
			return preset.Bindings.ThumbwheelButtonLeft
		case gesture.ThumbwheelRight:
			return preset.Bindings.ThumbwheelButtonRight
		}
		return config.Action{}
	}
	switch direction {
	case gesture.ThumbwheelLeft:
		return preset.Bindings.ThumbwheelLeft
	case gesture.ThumbwheelRight:
		return preset.Bindings.ThumbwheelRight
	default:
		return config.Action{}
	}
}

func sideButtonAction(preset config.Preset, code evdev.EvCode) (config.Action, bool) {
	switch code {
	case evdev.BTN_SIDE:
		return preset.Bindings.SideBack, true
	case evdev.BTN_EXTRA:
		return preset.Bindings.SideForward, true
	default:
		return config.Action{}, false
	}
}

func thumbClickAction(preset config.Preset, code evdev.EvCode) (config.Action, bool) {
	switch code {
	case evdev.BTN_LEFT:
		return preset.Bindings.ThumbLeftClick, true
	case evdev.BTN_MIDDLE:
		return preset.Bindings.ThumbMiddleClick, true
	case evdev.BTN_RIGHT:
		return preset.Bindings.ThumbRightClick, true
	default:
		return config.Action{}, false
	}
}

func handleThumbClickEvent(
	preset config.Preset,
	event *evdev.InputEvent,
	thumbGestureActive bool,
	consumed map[evdev.EvCode]bool,
	dispatch func(config.Action),
) bool {
	if event.Type != evdev.EV_KEY {
		return false
	}
	if consumed[event.Code] {
		if event.Value == 0 {
			delete(consumed, event.Code)
		}
		return true
	}
	if event.Value != 1 || !thumbGestureActive {
		return false
	}
	action, supported := thumbClickAction(preset, event.Code)
	if !supported || strings.TrimSpace(action.Value) == "" {
		return false
	}
	consumed[event.Code] = true
	dispatch(action)
	return true
}

func handleSideButtonEvent(
	preset config.Preset,
	event *evdev.InputEvent,
	consumed map[evdev.EvCode]bool,
	dispatch func(config.Action),
) bool {
	if event.Type != evdev.EV_KEY {
		return false
	}
	if consumed[event.Code] {
		if event.Value == 0 {
			delete(consumed, event.Code)
		}
		return true
	}
	if event.Value != 1 {
		return false
	}
	action, supported := sideButtonAction(preset, event.Code)
	if !supported || strings.TrimSpace(action.Value) == "" {
		return false
	}
	consumed[event.Code] = true
	dispatch(action)
	return true
}

func thumbwheelActionCount(
	code evdev.EvCode,
	value int32,
	remainders map[thumbwheelRemainderKey]int64,
	remainderKey thumbwheelRemainderKey,
) (int, gesture.InputKind) {
	direction, ok := horizontalWheelDirection(code, value)
	if !ok {
		return 0, 0
	}
	if code == evdev.REL_HWHEEL_HI_RES {
		total := remainders[remainderKey] + int64(value)
		count := total / 120
		remainders[remainderKey] = total - count*120
		if count < 0 {
			direction = gesture.ThumbwheelLeft
			count = -count
		}
		if count > 32 {
			count = 32
		}
		return int(count), direction
	}
	count := int64(value)
	if count < 0 {
		count = -count
	}
	if count > 32 {
		count = 32
	}
	return int(count), direction
}

func pollFocusedAppID(ctx context.Context, client *niri.Client, updates chan string) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastLoggedError string
	var lastErrorLog time.Time
	for {
		focusCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		appID, err := client.FocusedAppID(focusCtx)
		cancel()
		if err == nil {
			select {
			case updates <- appID:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- appID:
				default:
				}
			}
			lastLoggedError = ""
		} else if err.Error() != "Niri reports no focused window" {
			if err.Error() != lastLoggedError || time.Since(lastErrorLog) >= 5*time.Second {
				log.Printf("refresh focused app for thumbwheel: %v", err)
				lastLoggedError = err.Error()
				lastErrorLog = time.Now()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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

func dispatchActions(client *niri.Client, actions []config.Action) {
	for _, action := range actions {
		switch action.Type {
		case "", config.ActionNiri:
			args, err := actionArguments(action)
			if err != nil {
				log.Printf("parse Niri action %q: %v", action.Value, err)
				continue
			}
			client.ActionArgs(args)
		case config.ActionNoctalia:
			args, err := actionArguments(action)
			if err != nil {
				log.Printf("parse Noctalia action %q: %v", action.Value, err)
				continue
			}
			if len(args) == 0 {
				continue
			}
			go runGestureCommand("noctalia", append([]string{"msg"}, args...)...)
		case config.ActionCommand:
			go runGestureShellCommand(action.Value)
		}
	}
}

func actionArguments(action config.Action) ([]string, error) {
	args, err := splitActionArguments(action.Value)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, nil
	}
	additional, err := splitActionArguments(action.Arguments)
	if err != nil {
		return nil, err
	}
	return append(args, additional...), nil
}

func splitActionArguments(value string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	inArgument := false

	for _, char := range value {
		switch {
		case escaped:
			current.WriteRune(char)
			escaped = false
			inArgument = true
		case char == '\\' && quote != '\'':
			escaped = true
			inArgument = true
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
		case char == '\'' || char == '"':
			quote = char
			inArgument = true
		case char == ' ' || char == '\t' || char == '\n':
			if inArgument {
				args = append(args, current.String())
				current.Reset()
				inArgument = false
			}
		default:
			current.WriteRune(char)
			inArgument = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("trailing escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inArgument {
		args = append(args, current.String())
	}
	return args, nil
}

func runGestureCommand(name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("run gesture command %q: %v: %s", name, err, strings.TrimSpace(string(output)))
	}
}

func runGestureShellCommand(command string) {
	if strings.TrimSpace(command) == "" {
		return
	}
	runGestureCommand("sh", "-c", command)
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
