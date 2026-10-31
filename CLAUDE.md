# MX Mouse development notes

`nnmxd` (Noctalia Niri MX daemon) is a native user service for Niri and Logitech MX Master 3/3S thumb-button input. It fully replaces the earlier `mxmoused` program. Do not execute, depend on, or restore the legacy binary, system service, YAML config, or logid integration.

## Architecture

- `internal/device` finds an explicitly configured evdev path or auto-detects a Logitech mouse with the configured trigger-button capability.
- `internal/service` grabs the input device, clones ordinary pointer events through uinput, injects supported virtual modifier keys, reloads validated JSON config on trigger presses, and publishes runtime status.
- `internal/gesture` is a pure gesture state machine. It recognizes tap, directional movement, escalating repeat steps, and scroll while held.
- `internal/niri` reads the focused window's exact `app_id` from Niri JSON and dispatches Niri actions.
- `internal/config` owns defaults, strict JSON parsing, validation, exact app-id preset selection, and device-independent preset types.
- `cmd/nnmxd --apply-config <candidate>` validates a staged config and atomically publishes it. The Noctalia panel uses this instead of editing the active file in-place.

The `ffngs/mxmouse` plugin is in the sibling `community-plugins` checkout. It writes the config at `$XDG_CONFIG_HOME/mxmouse/config.json` (or `~/.config/mxmouse/config.json`) and reads the service status at `$XDG_RUNTIME_DIR/mxmouse/status.json`.

## Development

```sh
go test ./...
go build -o /tmp/nnmxd ./cmd/nnmxd
```

Tests must not grab physical input devices or inject events. Keep gesture behavior testable as a pure engine.

The user service requires the current user to have access to the mouse's evdev node and `/dev/uinput`. Do not add root escalation or a privileged system service; document the input-group permission implications instead.
