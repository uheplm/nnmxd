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

type WindowGeometry struct {
	AppID        string
	WindowWidth  float64
	WindowHeight float64
	OutputWidth  float64
	OutputHeight float64
}

type focusedWindowInfo struct {
	AppID  string `json:"app_id"`
	Layout struct {
		WindowSize []float64 `json:"window_size"`
	} `json:"layout"`
}

type focusedOutputInfo struct {
	Logical *struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"logical"`
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

func (c *Client) FocusedWindowGeometry(ctx context.Context) (WindowGeometry, error) {
	windowJSON, err := c.command(ctx, "--json", "focused-window")
	if err != nil {
		return WindowGeometry{}, err
	}
	outputJSON, err := c.command(ctx, "--json", "focused-output")
	if err != nil {
		return WindowGeometry{}, err
	}
	geometry, err := parseFocusedWindowGeometry(windowJSON, outputJSON)
	if err != nil {
		return WindowGeometry{}, err
	}
	return geometry, nil
}

func parseFocusedWindowGeometry(windowJSON, outputJSON []byte) (WindowGeometry, error) {
	var window focusedWindowInfo
	if err := json.Unmarshal(windowJSON, &window); err != nil {
		return WindowGeometry{}, fmt.Errorf("decode Niri focused window: %w", err)
	}
	if window.AppID == "" {
		return WindowGeometry{}, errors.New("Niri focused window has no app_id")
	}
	if len(window.Layout.WindowSize) < 2 ||
		window.Layout.WindowSize[0] <= 0 ||
		window.Layout.WindowSize[1] <= 0 {
		return WindowGeometry{}, errors.New("Niri focused window has no positive dimensions")
	}

	var output focusedOutputInfo
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		return WindowGeometry{}, fmt.Errorf("decode Niri focused output: %w", err)
	}
	if output.Logical == nil || output.Logical.Width <= 0 || output.Logical.Height <= 0 {
		return WindowGeometry{}, errors.New("Niri focused output has no positive logical dimensions")
	}
	return WindowGeometry{
		AppID:        window.AppID,
		WindowWidth:  window.Layout.WindowSize[0],
		WindowHeight: window.Layout.WindowSize[1],
		OutputWidth:  output.Logical.Width,
		OutputHeight: output.Logical.Height,
	}, nil
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
