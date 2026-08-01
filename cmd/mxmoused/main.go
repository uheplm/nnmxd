// mxmoused — userspace daemon that repurposes the Logitech MX Master 3 / 3S
// thumb button on niri (Wayland). Selects between two modes at startup based
// on config.yaml: gesture navigation or meta-key toggle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/holoplot/go-evdev"

	"github.com/naudachu/mxmoused/internal/action"
	"github.com/naudachu/mxmoused/internal/config"
	"github.com/naudachu/mxmoused/internal/device"
	"github.com/naudachu/mxmoused/internal/logid"
	"github.com/naudachu/mxmoused/internal/modes/gesture"
	"github.com/naudachu/mxmoused/internal/modes/metatoggle"
	"github.com/naudachu/mxmoused/internal/niri"
)

func main() {
	log.SetFlags(log.Ltime)

	var configPath string
	flag.StringVar(&configPath, "config", "",
		"path to config.yaml (default: $XDG_CONFIG_HOME/mxmoused/config.yaml, then /etc/mxmoused/config.yaml)")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	dev, err := device.Open(cfg.Device)
	if err != nil {
		log.Fatalf("device: %v", err)
	}
	defer dev.Close()

	name, _ := dev.Name()
	log.Printf("device: %q (%s)", name, dev.Path())
	log.Printf("mode: %s", cfg.Mode)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutting down")
		cancel()
	}()

	if err := run(ctx, cfg, dev); err != nil {
		log.Fatalf("%s mode: %v", cfg.Mode, err)
	}
}

func run(ctx context.Context, cfg *config.Config, dev *evdev.InputDevice) error {
	switch cfg.Mode {
	case config.ModeGesture:
		return runGesture(ctx, cfg, dev)
	case config.ModeMetaToggle:
		return metatoggle.Run(ctx, dev, cfg.MetaToggle, cfg.Device.TriggerButton)
	default:
		return fmt.Errorf("unknown mode %q", cfg.Mode)
	}
}

func runGesture(ctx context.Context, cfg *config.Config, dev *evdev.InputDevice) error {
	n := niri.New("")

	logidClient, err := logid.New(ctx)
	if err != nil {
		// System bus unreachable — continue without logid. Any logid:*
		// actions in config will be dropped with a warning.
		log.Printf("logid: %v (continuing without logid backend)", err)
		return gesture.Run(ctx, dev, action.New(n, nil), cfg.Gesture, cfg.Device.TriggerButton)
	}
	defer logidClient.Close()

	if logidClient.Available() {
		log.Printf("logid: available")
	} else {
		log.Printf("logid: not running (logid:* actions will activate if it comes up)")
	}

	backend := action.NewLogidDeviceBackend(logidClient, logidDeviceNames(cfg.Logid))
	go backend.WatchPresence(ctx)

	return gesture.Run(ctx, dev, action.New(n, backend), cfg.Gesture, cfg.Device.TriggerButton)
}

func logidDeviceNames(cfg config.LogidConfig) []string {
	if cfg.DeviceName != "" {
		return []string{cfg.DeviceName}
	}
	return []string{"MX Master 3S", "MX Master 3"}
}
