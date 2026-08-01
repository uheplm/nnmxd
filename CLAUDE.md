# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`mxmoused` is a Linux userspace daemon in Go. Current scope: repurposes the Logitech MX Master 3 / 3S thumb button for the niri Wayland compositor via raw evdev + uinput. One binary, two runtime modes selected at startup from `config.yaml`: `gesture` and `meta-toggle`.

**Direction**: niri/Wayland-oriented superstructure for Logitech mice. **Not** a Go rewrite of [LogiOps](https://github.com/PixlOne/logiops) — `logid` is treated as a soft runtime dependency. When `logid` is present, HID++ features (SmartShift, HiResScroll, DPI, ChangeHost, profiles) are delegated to it via D-Bus (`pizza.pixl.LogiOps`). When absent, only compositor-level features work. See `ROADMAP.md`.

**Two classes of features:**

- **Augmenting** — work without `logid`; extend if present. Example: per-app profiles work via our own binding swap, and *additionally* call logid's `SetProfile` when available. No hard error if logid is missing.
- **logid-only** — have no meaning without `logid`. Bindings prefixed with `logid:` (e.g., `logid:cycle-dpi`, `logid:toggle-smartshift`). If such a binding appears in config while logid is not running, it is silently ignored with a warning — the daemon does not fail. If logid comes up later (detected via `NameOwnerChanged`), those bindings become active without restart.

Never reimplement HID++ ourselves. If we need to read or write HID++, route through `logid`.

## Build & run

For a one-off dev run:

```sh
go build -o bin/mxmoused ./cmd/mxmoused
sudo ./bin/mxmoused --config ./configs/mxmoused.example.yaml
```

For the normal install/edit loop, use the `Makefile`:

```sh
make install     # build + ~/.local/bin + /etc/systemd/system/mxmoused.service + enable --now
make reinstall   # rebuild + swap binary + systemctl restart  (the inner loop)
make logs        # journalctl -u mxmoused -f
make uninstall
```

Root is required: the daemon grabs an evdev input device and creates a `uinput` clone / virtual keyboard. The unit runs as a system service; only the unit-install and `systemctl` calls in the Makefile use `sudo`.

Binary path is `~/.local/bin/mxmoused`, matching `configs/mxmoused.service`. The unit globs `/run/user/*/niri.*.sock` from root, so niri IPC works without an explicit `NIRI_SOCKET=` env.

## Tests

```sh
go test ./...                                    # all packages
go test ./internal/modes/gesture -run TestEngine # single package / filter
```

Tests that hit evdev / uinput are intentionally avoided — only pure-logic packages (`config`, `modes/gesture` engine, `modes/metatoggle` state builder) are unit-tested. Don't try to mock evdev; it's kernel-facing.

## Architecture

Entry point `cmd/mxmoused/main.go` loads config, opens the device, then dispatches to exactly one of two mode packages. The two modes handle the device **differently** — this is the most important split to understand:

- **`internal/modes/gesture`** — calls `dev.Grab()` (exclusive). All real-device events are intercepted; a `uinput` clone (`CloneDevice`) re-emits the ones we want the compositor to see (clicks, normal scroll, SYN/MSC). While the trigger button is held, `EV_REL` events are swallowed so the cursor doesn't drift, and fed to the gesture engine instead. Engine output is a list of action names passed to a `Dispatcher` interface (see below) — gesture mode does **not** know about niri or logid directly.
- **`internal/modes/metatoggle`** — does **not** grab the device. Reads passively so the compositor still sees every mouse event natively. Creates a minimal virtual keyboard with a single key, and latches that key on/off on each trigger press. Optionally writes a Waybar-compatible JSON state file.

The gesture engine (`internal/modes/gesture/engine.go`) is a pure state machine over abstract `Input` values (`InputButtonPress`, `InputRelX/Y`, `InputScroll`, …). `runtime.go` is a thin evdev adapter that feeds it. Keep this split when editing — gesture logic is unit-tested without touching the kernel.

### Action dispatcher (`internal/action`)

The gesture engine emits action names as plain strings. `internal/action.Dispatcher` routes them by prefix:

- bare names (`focus-column-left`, etc.) → `niri.Client.Action` → `niri msg action <name>` subprocess
- `logid:*` names → `LogidBackend` → D-Bus call to `pizza.pixl.LogiOps`

Currently supported `logid:*` actions: `logid:cycle-profile`, `logid:set-profile:<name>`. Unknown `logid:*` names and presence-down conditions go through `warnOnce` so a held gesture doesn't spam the log.

`LogidDeviceBackend` (in the same package) caches a resolved `*logid.Device` and invalidates it on every `PresenceUp/Down` from `logid.Client.Watch()` — so `systemctl restart logid` recovers without restarting mxmoused. Wiring lives in `cmd/mxmoused/main.go:runGesture`.

When adding new dispatch destinations (e.g. UPower battery actions), extend `Dispatcher` rather than calling out from the engine. The `Dispatcher` interface in `internal/modes/gesture/runtime.go` is intentionally a single-method seam.

Step thresholds in the engine escalate within a single hold: 1st step fires at `threshold` pixels, 2nd at `threshold + escalation`, 3rd at `threshold + 2*escalation`, etc. Resetting happens on button release, not between steps within the same hold.

`internal/device/device.go` finds the mouse by Logitech vendor id `0x046d` **plus** the presence of the configured trigger button capability. This is deliberate — the USB receiver exposes a keyboard-shaped sub-device under the same vendor id that must be rejected. Works for both USB receiver and Bluetooth (BLE names it `MX Master 3` / `3S`, USB names it `Logitech USB Receiver Mouse`).

`internal/niri/niri.go` talks to niri via the `niri msg` CLI (not the socket directly) to stay decoupled from niri's IPC protocol. Actions are fired in goroutines and not awaited by the caller.

## Config

`internal/config/config.go` loads YAML over `Defaults()`, so omitted keys keep their built-in defaults. Search order for `--config` unset: `$XDG_CONFIG_HOME/mxmoused/config.yaml` → `~/.config/mxmoused/config.yaml` → `/etc/mxmoused/config.yaml` → built-in defaults only (no file). `Validate()` is always called after load — add new required-field checks there, not in `Load`.

`logid.device_name` selects which device block in `logid.cfg` we drive over D-Bus. When empty, the daemon tries `MX Master 3S` then `MX Master 3` in order (see `cmd/mxmoused/main.go:logidDeviceNames`).

## logid integration & gotcha

`logid` is a **soft** dependency — the daemon runs without it; `logid:*` bindings then drop with a single warning per name. `internal/logid` watches `org.freedesktop.DBus.NameOwnerChanged` so logid coming up after mxmoused is fine.

**Critical gotcha — do not list the trigger button cid in `logid.cfg` `buttons:` block.** Putting *any* entry for cid `0xc3` (the MX Master 3/3S thumb button) in `buttons:` causes logid to divert it via HID++. Even `action = { type: "None"; }` (NullAction) **swallows** the event — it never reaches evdev, and mxmoused sees nothing. The fix is to omit the entry entirely. See `configs/logid.cfg.example` for the correct shape.

Mirror any change to `/etc/logid.cfg` (or other neighbouring system configs we depend on) into `configs/` so it stays tracked. Naming: `.example` suffix for reference templates the user adapts (`logid.cfg.example`, `mxmoused.example.yaml`); no suffix for files the install step copies as-is (`mxmoused.service`).
