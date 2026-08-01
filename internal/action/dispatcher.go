// Package action routes gesture-engine action names to the correct backend.
// Names prefixed with "logid:" are dispatched to the logid D-Bus client;
// everything else goes to niri via "niri msg action". Dispatch is fire-and-
// forget — slow D-Bus calls do not stall the event loop.
package action

import (
	"log"
	"strings"
	"sync"
)

const logidPrefix = "logid:"

// NiriClient is the subset of *niri.Client that Dispatcher uses. Keeping
// this as an interface lets tests drop in a fake.
type NiriClient interface {
	Action(name string)
}

// LogidBackend is what Dispatcher needs to drive logid. Implementations
// handle device (re-)resolution and presence transitions internally.
type LogidBackend interface {
	// Available reports whether logid is reachable right now. Used to
	// short-circuit warnings for logid:* actions when logid is absent.
	Available() bool
	// SetProfile switches the active logid profile.
	SetProfile(name string) error
	// CycleProfile rotates to the next configured profile.
	CycleProfile() error
}

// Dispatcher splits action names by prefix and hands them off to niri or
// logid. Safe for concurrent use.
type Dispatcher struct {
	niri  NiriClient
	logid LogidBackend

	warnedOnce sync.Map
}

// New builds a Dispatcher. logid may be nil — logid:* actions will then
// log a single warning per distinct action name and be dropped.
func New(n NiriClient, l LogidBackend) *Dispatcher {
	return &Dispatcher{niri: n, logid: l}
}

// Dispatch routes one action name to the appropriate backend.
func (d *Dispatcher) Dispatch(name string) {
	if name == "" {
		return
	}
	if rest, ok := strings.CutPrefix(name, logidPrefix); ok {
		d.dispatchLogid(name, rest)
		return
	}
	d.niri.Action(name)
}

func (d *Dispatcher) dispatchLogid(full, rest string) {
	if d.logid == nil {
		d.warnOnce(full, "logid backend not configured")
		return
	}
	if !d.logid.Available() {
		d.warnOnce(full, "logid not running")
		return
	}

	switch {
	case rest == "cycle-profile":
		go func() {
			if err := d.logid.CycleProfile(); err != nil {
				log.Printf("logid: cycle-profile: %v", err)
			}
		}()
	case strings.HasPrefix(rest, "set-profile:"):
		profile := strings.TrimPrefix(rest, "set-profile:")
		if profile == "" {
			d.warnOnce(full, "empty profile name")
			return
		}
		go func() {
			if err := d.logid.SetProfile(profile); err != nil {
				log.Printf("logid: set-profile %q: %v", profile, err)
			}
		}()
	default:
		d.warnOnce(full, "unknown logid action")
	}
}

// warnOnce prints a warning the first time it sees a given action name and
// suppresses repeats — configs often fire the same unknown binding many
// times per gesture hold, and we do not want to spam the log.
func (d *Dispatcher) warnOnce(key, reason string) {
	if _, loaded := d.warnedOnce.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	log.Printf("action %q ignored: %s", key, reason)
}
