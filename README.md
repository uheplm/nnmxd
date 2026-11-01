# nnmxd

`nnmxd` (Noctalia Niri MX daemon) is a native user service for Logitech MX Master 3/3S thumb-button gestures and modifier toggling in Niri. It reads Linux input events directly, clones the pointer through uinput, dispatches Niri actions, and chooses presets using the focused window's exact `app_id`. It does not execute or depend on the old `mxmoused` program, and it does not depend on logid.

The companion `ffngs/mxmouse` Noctalia plugin provides a panel for service status, device settings, gesture bindings, and per-application preset assignments.

## Features

- Hold the configured thumb button and move to dispatch directional Niri actions. The movement threshold controls the first action; retrigger distance controls movement between later actions and defaults to the threshold. Escalation adds to that distance after each retrigger and defaults to 0.
- Tap the button to dispatch the preset's tap action.
- Hold and scroll to dispatch scroll-up/down actions.
- Presets can use gesture or meta-toggle mode. Meta-toggle injects a configured modifier key on each press and releases all held virtual keys when the service stops.
- Assign a preset to each exact Niri `app_id`; all other apps use the configured default preset.
- The pointer device is grabbed while the service runs and all non-gesture events are forwarded through a virtual clone.
- Set a mouse's native HID++ adjustable DPI without Solaar, if the paired mouse supports the feature and the user can access its receiver's `/dev/hidraw*` node. DPI `0` leaves the hardware setting unchanged; otherwise, nnmxd applies the requested supported value when starting and on the next thumb-button press. Some devices may have onboard profile settings that override the host-set DPI.
- Independently scale vertical and horizontal standard and high-resolution wheel events from 10% to 500%; the default is 100%.
- Optional Waybar-compatible JSON state file for modifier toggle state.

## Input permissions

This is a user service, not a root daemon. The running user needs read/write access to the mouse's `/dev/input/event*` node and `/dev/uinput`. To configure DPI, the user also needs read/write access to the receiver's HID++ `/dev/hidraw*` node. On common Linux distributions the input nodes are owned by `root:input` with group read/write permissions; hidraw permissions vary by distribution and may need a narrowly scoped udev rule. Add your user to the `input` group, then fully log out and back in:

```sh
sudo usermod -aG input "$USER"
```

Check permissions before starting:

```sh
ls -l /dev/input/by-id /dev/uinput
```

The `input` group can access every input device on the system, not just the mouse. If that is too broad for your setup, use a narrowly scoped udev rule/group for the Logitech input node and uinput instead. The service will report permission errors in its status and journal; it does not elevate privileges.

## Install

Install Git, Make, Go, and the Linux uinput kernel module, ensure the input permissions above are in place, then:

```sh
make install
```

This builds `nnmxd`, installs the binary in `~/.local/bin`, installs and enables `nnmxd.service`, and starts the user service. To update after changing the source:

```sh
make reinstall
```

Useful commands:

```sh
make status
make logs
make stop
make start
make uninstall
```

The service writes its runtime status to `$XDG_RUNTIME_DIR/mxmouse/status.json`. Its configuration is stored in `$XDG_CONFIG_HOME/mxmouse/config.json`, or `~/.config/mxmouse/config.json` when `XDG_CONFIG_HOME` is unset. These paths retain the MX Mouse product namespace. The Noctalia panel writes configuration through nnmxd's validated, atomic `--apply-config` operation. After a panel save, press the thumb button once to apply it; nnmxd consumes that press without running its gesture or toggle action and reports the applied generation in runtime status. Changing device detection, device path, or trigger button still requires restarting the service.

The Noctalia plugin can install nnmxd from source into `~/.local/src/nnmxd` by running `make install`. This compiles on the user's machine and downloads Go module dependencies through Go; no compiled dependency bundle is shipped. To install manually:

```sh
git clone https://github.com/uheplm/nnmxd.git ~/.local/src/nnmxd
make -C ~/.local/src/nnmxd install
```

If the legacy system-level `mxmoused.service` is still installed, stop it before starting nnmxd so the two daemons do not contend for the same input device:

```sh
sudo systemctl disable --now mxmoused.service
```

## Noctalia plugin

Install `ffngs/mxmouse` from the community catalog. If nnmxd is missing, the panel offers the source-build installation wizard. Open the **MX Mouse** panel from the plugin entry or:

```sh
noctalia msg panel-toggle ffngs/mxmouse:manager
```

Create and edit presets in the panel. Choose gesture or meta-toggle mode; gesture bindings are Niri action names, such as `focus-column-left` or `toggle-overview`. In the per-application list, **Next preset** cycles between the default and named presets for that exact app ID. Set DPI to `0` to leave the current hardware setting unchanged, or choose a supported DPI value; scroll scales are independently adjustable by axis. Use **Save configuration** to validate and write changes. Use **Restart input service** after changing device options.

The focused `app_id` comes from `niri msg --json windows`. Window title is displayed for identification but does not participate in preset matching. Niri and nnmxd must both be installed; the daemon is normally installed at `~/.local/bin/nnmxd`.

## Development

```sh
make test
make build
```

The gesture engine is a pure state machine with unit tests. Evdev and uinput need a real Linux input stack, so tests do not grab the mouse or inject events.

## License

MIT
