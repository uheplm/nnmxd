package action

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/naudachu/mxmoused/internal/logid"
)

// LogidDeviceBackend implements LogidBackend by resolving a *logid.Device
// on demand and driving profile operations through it. The cached handle
// is invalidated on any ErrUnavailable or on logid presence transitions,
// so a logid restart recovers without a daemon restart.
type LogidDeviceBackend struct {
	client      *logid.Client
	deviceNames []string

	mu     sync.Mutex
	cached *logid.Device
}

// NewLogidDeviceBackend wires a presence-aware logid backend. If client is
// nil (e.g. the system bus was unreachable at startup), Available always
// reports false and every method returns logid.ErrUnavailable. Names are
// tried in order during device resolution; the first match wins.
func NewLogidDeviceBackend(client *logid.Client, deviceNames []string) *LogidDeviceBackend {
	return &LogidDeviceBackend{
		client:      client,
		deviceNames: deviceNames,
	}
}

// Available reports whether logid currently owns its bus name.
func (b *LogidDeviceBackend) Available() bool {
	return b.client != nil && b.client.Available()
}

// WatchPresence invalidates the cached device on every presence transition
// so the next call re-resolves against the fresh object tree. Returns when
// ctx is cancelled or the client's Watch channel is closed.
func (b *LogidDeviceBackend) WatchPresence(ctx context.Context) {
	if b.client == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-b.client.Watch():
			if !ok {
				return
			}
			switch p {
			case logid.PresenceUp:
				log.Printf("logid: available")
			case logid.PresenceDown:
				log.Printf("logid: unavailable")
			}
			b.invalidate()
		}
	}
}

// SetProfile resolves the device (if needed) and switches to the named profile.
func (b *LogidDeviceBackend) SetProfile(name string) error {
	dev, err := b.resolve()
	if err != nil {
		return err
	}
	if err := dev.SetProfile(name); err != nil {
		b.handleCallErr(err)
		return err
	}
	return nil
}

// CycleProfile rotates to the next profile in GetProfiles order.
func (b *LogidDeviceBackend) CycleProfile() error {
	dev, err := b.resolve()
	if err != nil {
		return err
	}
	profiles, err := dev.GetProfiles()
	if err != nil {
		b.handleCallErr(err)
		return err
	}
	if len(profiles) < 2 {
		return fmt.Errorf("logid: only %d profile(s) configured; nothing to cycle", len(profiles))
	}
	active, err := dev.ActiveProfile()
	if err != nil {
		b.handleCallErr(err)
		return err
	}
	next := nextProfile(profiles, active)
	if err := dev.SetProfile(next); err != nil {
		b.handleCallErr(err)
		return err
	}
	return nil
}

func (b *LogidDeviceBackend) resolve() (*logid.Device, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cached != nil {
		return b.cached, nil
	}
	if b.client == nil || !b.client.Available() {
		return nil, logid.ErrUnavailable
	}
	var lastErr error
	for _, name := range b.deviceNames {
		dev, err := b.client.FindDevice(name)
		if err == nil {
			b.cached = dev
			return dev, nil
		}
		lastErr = err
		if errors.Is(err, logid.ErrUnavailable) {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("logid: no matching device (tried %v): %w", b.deviceNames, lastErr)
	}
	return nil, fmt.Errorf("logid: no device names configured")
}

func (b *LogidDeviceBackend) invalidate() {
	b.mu.Lock()
	b.cached = nil
	b.mu.Unlock()
}

func (b *LogidDeviceBackend) handleCallErr(err error) {
	if errors.Is(err, logid.ErrUnavailable) {
		b.invalidate()
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
