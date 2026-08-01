// logid-smoke is a throwaway manual-verification binary for the
// internal/logid client against a live logid daemon. Not part of mxmoused;
// delete after verification.
//
// Run as root:
//
//	sudo go run ./cmd/logid-smoke
//	sudo go run ./cmd/logid-smoke --rotate  // rotate to next profile (>1 profile in logid.cfg)
//
// Exercises: New, Available, WaitReady, FindDevice, GetProfiles,
// ActiveProfile, (optionally) SetProfile, Watch. To observe presence
// transitions, leave this running and run `sudo systemctl restart logid`
// in another shell.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/naudachu/mxmoused/internal/logid"
)

func main() {
	log.SetFlags(log.Ltime)

	deviceName := flag.String("device", "MX Master 3S", "logid device name")
	rotate := flag.Bool("rotate", false, "rotate to next profile if >1 is configured")
	waitTimeout := flag.Duration("wait", 30*time.Second, "how long to wait for logid if it's down at startup")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutting down")
		cancel()
	}()

	c, err := logid.New(ctx)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	defer c.Close()

	log.Printf("Available() = %v", c.Available())

	go func() {
		for p := range c.Watch() {
			switch p {
			case logid.PresenceUp:
				log.Printf("presence: UP")
			case logid.PresenceDown:
				log.Printf("presence: DOWN")
			}
		}
	}()

	if !c.Available() {
		log.Printf("logid not running; waiting up to %s for it to come up", *waitTimeout)
		waitCtx, waitCancel := context.WithTimeout(ctx, *waitTimeout)
		if err := c.WaitReady(waitCtx); err != nil {
			waitCancel()
			log.Fatalf("WaitReady: %v", err)
		}
		waitCancel()
	}

	dev, err := c.FindDevice(*deviceName)
	if err != nil {
		log.Fatalf("FindDevice(%q): %v", *deviceName, err)
	}
	log.Printf("device: path=%s name=%q", dev.Path(), dev.Name())

	profiles, err := dev.GetProfiles()
	if err != nil {
		log.Fatalf("GetProfiles: %v", err)
	}
	log.Printf("profiles: %v", profiles)

	active, err := dev.ActiveProfile()
	if err != nil {
		log.Fatalf("ActiveProfile: %v", err)
	}
	log.Printf("active profile: %q", active)

	switch {
	case !*rotate:
		log.Printf("skipping rotate (pass --rotate to exercise SetProfile)")
	case len(profiles) < 2:
		log.Printf("only %d profile configured; add a second one to logid.cfg to exercise rotate", len(profiles))
	default:
		next := nextProfile(profiles, active)
		log.Printf("rotating: %q -> %q", active, next)
		if err := dev.SetProfile(next); err != nil {
			log.Fatalf("SetProfile(%q): %v", next, err)
		}
		got, err := dev.ActiveProfile()
		if err != nil {
			log.Fatalf("ActiveProfile after set: %v", err)
		}
		log.Printf("active profile now: %q", got)
	}

	log.Printf("idle — try 'sudo systemctl restart logid' to see presence events; Ctrl-C to exit")
	<-ctx.Done()

	if _, err := dev.ActiveProfile(); errors.Is(err, logid.ErrUnavailable) {
		log.Printf("post-cancel ActiveProfile -> ErrUnavailable (expected if logid is down or client has closed)")
	}
}

func nextProfile(profiles []string, current string) string {
	for i, p := range profiles {
		if p == current {
			return profiles[(i+1)%len(profiles)]
		}
	}
	return profiles[0]
}
