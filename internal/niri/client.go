package niri

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Window struct {
	ID        int    `json:"id"`
	AppID     string `json:"app_id"`
	Title     string `json:"title"`
	IsFocused bool   `json:"is_focused"`
}

type Client struct {
	socket string
}

func New() *Client {
	socket := os.Getenv("NIRI_SOCKET")
	if socket == "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir != "" {
			matches, _ := filepath.Glob(filepath.Join(runtimeDir, "niri.*.sock"))
			if len(matches) > 0 {
				socket = matches[0]
			}
		}
	}
	return &Client{socket: socket}
}

func (c *Client) FocusedAppID(ctx context.Context) (string, error) {
	output, err := c.command(ctx, "--json", "windows")
	if err != nil {
		return "", err
	}
	var windows []Window
	if err := json.Unmarshal(output, &windows); err != nil {
		return "", fmt.Errorf("decode Niri windows: %w", err)
	}
	for _, window := range windows {
		if window.IsFocused {
			return window.AppID, nil
		}
	}
	return "", errors.New("Niri reports no focused window")
}

func (c *Client) Action(name string) {
	c.ActionArgs([]string{name})
}

func (c *Client) ActionArgs(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return
	}
	args = append([]string(nil), args...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := c.command(ctx, append([]string{"action"}, args...)...); err != nil {
			fmt.Fprintf(os.Stderr, "niri action %q failed: %v\n", strings.Join(args, " "), err)
		}
	}()
}

func (c *Client) command(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "niri", append([]string{"msg"}, args...)...)
	if c.socket != "" {
		cmd.Env = append(os.Environ(), "NIRI_SOCKET="+c.socket)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return nil, fmt.Errorf("run niri msg %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("run niri msg %s: %w: %s", strings.Join(args, " "), err, detail)
	}
	return output, nil
}
