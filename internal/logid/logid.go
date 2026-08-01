// Package logid is a minimal D-Bus client for the logiops system daemon
// (bus name pizza.pixl.LogiOps). logid is a soft dependency: New always
// succeeds as long as the system bus itself is reachable, and presence
// transitions are delivered via Watch. Methods that need logid return
// ErrUnavailable while the service is down.
package logid

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	busName     = "pizza.pixl.LogiOps"
	devicesPath = dbus.ObjectPath("/pizza/pixl/logiops/devices")
	deviceIface = "pizza.pixl.LogiOps.Device"
)

// Presence is delivered on Client.Watch whenever logid's ownership of its
// well-known bus name changes.
type Presence int

const (
	PresenceDown Presence = iota
	PresenceUp
)

// Client owns a private system-bus connection, tracks logid's presence via
// NameOwnerChanged, and exposes high-level operations against the device
// tree. Safe for concurrent use.
type Client struct {
	conn      *dbus.Conn
	available atomic.Bool

	watchCh chan Presence
	sigCh   chan *dbus.Signal

	matchOpts []dbus.MatchOption
	cancel    context.CancelFunc
	done      chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// New opens a private system-bus connection, installs a NameOwnerChanged
// match rule scoped to pizza.pixl.LogiOps, and starts the presence loop.
// It does not fail on "logid absent" — only if the system bus itself is
// unreachable or the match rule cannot be installed.
func New(ctx context.Context) (*Client, error) {
	_ = ctx // reserved for future context-aware construction

	conn, err := dbus.SystemBusPrivate()
	if err != nil {
		return nil, fmt.Errorf("logid: connect system bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("logid: auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("logid: hello: %w", err)
	}

	matchOpts := []dbus.MatchOption{
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, busName),
	}
	if err := conn.AddMatchSignal(matchOpts...); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("logid: add match: %w", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		conn:      conn,
		watchCh:   make(chan Presence, 4),
		sigCh:     make(chan *dbus.Signal, 16),
		matchOpts: matchOpts,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	conn.Signal(c.sigCh)

	// Install the match first, then seed available. If a transition happens
	// between AddMatchSignal and the initial pingOwner, the signal will
	// arrive on sigCh and the loop will reconcile it.
	c.available.Store(c.pingOwner())

	go c.signalLoop(loopCtx)
	return c, nil
}

// Close stops the presence loop, removes the match rule, closes the bus
// connection, and closes the Watch channel. Idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		<-c.done

		if err := c.conn.RemoveMatchSignal(c.matchOpts...); err != nil && c.closeErr == nil {
			c.closeErr = fmt.Errorf("logid: remove match: %w", err)
		}
		c.conn.RemoveSignal(c.sigCh)
		if err := c.conn.Close(); err != nil && c.closeErr == nil {
			c.closeErr = fmt.Errorf("logid: close bus: %w", err)
		}
		close(c.watchCh)
	})
	return c.closeErr
}

// Available reports whether logid currently owns pizza.pixl.LogiOps. Cheap
// and lock-free; safe to call from any goroutine.
func (c *Client) Available() bool { return c.available.Load() }

// WaitReady blocks until Available reports true or ctx is done. Returns
// nil when logid becomes available, ctx.Err() on cancellation. If the
// client is closed while waiting, returns an error.
func (c *Client) WaitReady(ctx context.Context) error {
	if c.available.Load() {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-c.watchCh:
			if !ok {
				return fmt.Errorf("logid: client closed")
			}
			if p == PresenceUp {
				return nil
			}
		}
	}
}

// Watch returns a channel emitting Presence events for logid up/down
// transitions. Single-consumer: reading from multiple goroutines races.
// Buffered (size 4); on overflow the oldest pending event is dropped.
// Closed on Close.
func (c *Client) Watch() <-chan Presence { return c.watchCh }

func (c *Client) signalLoop(ctx context.Context) {
	defer close(c.done)
	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-c.sigCh:
			if !ok {
				return
			}
			if sig == nil || sig.Name != "org.freedesktop.DBus.NameOwnerChanged" {
				continue
			}
			name, _, newOwner, valid := decodeNameOwnerChanged(sig.Body)
			if !valid || name != busName {
				continue
			}
			up := newOwner != ""
			prev := c.available.Swap(up)
			if prev != up {
				c.emitPresence(up)
			}
		}
	}
}

// pingOwner asks the D-Bus daemon whether busName currently has an owner.
// Returns false on any error (including NameHasNoOwner, which we treat as
// a normal absence signal).
func (c *Client) pingOwner() bool {
	var owner string
	err := c.conn.BusObject().Call(
		"org.freedesktop.DBus.GetNameOwner", 0, busName,
	).Store(&owner)
	return err == nil && owner != ""
}

// emitPresence posts p on watchCh. If the consumer has fallen behind, the
// oldest pending event is discarded to make room. Called only from
// signalLoop, so writes are serialized.
func (c *Client) emitPresence(up bool) {
	p := PresenceDown
	if up {
		p = PresenceUp
	}
	select {
	case c.watchCh <- p:
	default:
		select {
		case <-c.watchCh:
		default:
		}
		select {
		case c.watchCh <- p:
		default:
		}
	}
}
