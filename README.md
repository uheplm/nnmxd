# mxmoused

Userspace daemon that repurposes the Logitech MX Master 3 / 3S thumb button on niri (Wayland).

Two modes, selectable in `config.yaml`:

- **gesture** — macOS-style gesture navigation. Tap the thumb button to toggle the niri overview; hold and drag to switch columns/workspaces; hold and scroll to switch workspaces.
- **meta-toggle** — press the thumb button to virtually hold the Super key so niri keybinds work without holding a keyboard key. Writes a Waybar-compatible JSON state file.

## Status

Early development. No release yet. Packaging (AUR, deb, rpm) is tracked but not built.

## Install

The repo ships a `Makefile` that handles the full lifecycle:

```sh
make install     # build + install binary + system unit + enable & start
make reinstall   # rebuild + restart  (the edit→try loop)
make logs        # follow journal
make status
make uninstall
```

Binary lands in `~/.local/bin/mxmoused`; the systemd unit at `/etc/systemd/system/mxmoused.service`. Root is required at runtime — the daemon grabs an evdev input device and creates a `uinput` clone — so the unit runs as a system service. `sudo` is invoked only for the unit-install and `systemctl` calls.

For a one-off dev run without installing:

```sh
go build -o bin/mxmoused ./cmd/mxmoused
sudo ./bin/mxmoused --config ./configs/mxmoused.example.yaml
```

## Config

See `configs/mxmoused.example.yaml`. At runtime, `mxmoused` looks for a config in this order:

1. `--config <path>` flag
2. `$XDG_CONFIG_HOME/mxmoused/config.yaml` (fallback: `~/.config/mxmoused/config.yaml`)
3. `/etc/mxmoused/config.yaml`

`make install` seeds `~/.config/mxmoused/config.yaml` from the example on first run and never overwrites an existing file.

## Optional: logid integration

`mxmoused` is **not** a LogiOps rewrite. HID++ features (SmartShift, hi-res scroll, DPI, profiles) are delegated to [`logid`](https://github.com/PixlOne/logiops) over D-Bus when it is running. `logid` is a soft dependency — without it, only compositor-level features work; `logid:*` bindings in your config drop silently with a single warning.

When `logid` is present you can use these action names in the gesture `bindings:` block:

- `logid:cycle-profile` — rotate to the next profile in `logid.cfg` order
- `logid:set-profile:<name>` — switch to a named profile

### `/etc/logid.cfg` gotcha

If you list the thumb-button cid (`0xc3` on MX Master 3/3S) in `logid.cfg`'s `buttons:` array, **logid swallows the event** — even with `action = { type: "None"; }`. The press never reaches evdev and `mxmoused` sees nothing. Listing the cid is itself the divert; the action only decides what happens *after* the divert.

Fix: omit the cid entirely. A working reference for an MX Master 3S lives at `configs/logid.cfg.example`. Apply with:

```sh
sudo cp configs/logid.cfg.example /etc/logid.cfg
sudo systemctl restart logid
```

## Supported hardware

- Logitech MX Master 3 and 3S
- Both USB receiver (`BUS_USB`) and Bluetooth (`BUS_BLUETOOTH`)
- Detected by vendor id `0x046d` + presence of the configured trigger button

## License

MIT
