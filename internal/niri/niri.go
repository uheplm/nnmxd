// Package niri is a minimal IPC wrapper that fires "niri msg action <name>"
// subprocesses. Using the CLI (not a direct socket client) keeps mxmoused
// decoupled from niri's internal IPC protocol.
package niri

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

type Client struct {
	socket string
}

// New returns a client. If socket is empty, the environment is checked
// ($NIRI_SOCKET), then /run/user/*/niri.*.sock is globbed. Either path may
// end up empty — in that case niri will rely on its own defaults.
func New(socket string) *Client {
	if socket == "" {
		socket = discoverSocket()
	}
	return &Client{socket: socket}
}

// Socket returns the resolved niri IPC socket path (may be empty).
func (c *Client) Socket() string { return c.socket }

// Action fires "niri msg action <name>" without blocking the caller.
// The process is waited on in a goroutine to avoid zombies.
func (c *Client) Action(name string) {
	go c.runAction(name)
}

func (c *Client) runAction(name string) {
	cmd := exec.Command("niri", "msg", "action", name)
	if c.socket != "" {
		cmd.Env = append(os.Environ(), "NIRI_SOCKET="+c.socket)
	}
	if err := cmd.Run(); err != nil {
		log.Printf("niri action %q: %v", name, err)
	}
}

func discoverSocket() string {
	if s := os.Getenv("NIRI_SOCKET"); s != "" {
		return s
	}
	matches, _ := filepath.Glob("/run/user/*/niri.*.sock")
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}
