# nnmxd

`nnmxd` (Noctalia Niri MX daemon) is a native user service for Logitech MX Master 3/3S thumb-button gestures, modifier toggling, and per-application mouse profiles. Its intended integration is the [MX Mouse Noctalia plugin](https://github.com/uheplm/ffngs-plugins/tree/main/mxmouse) with the Niri compositor.

Niri provides the focused-window `app_id` and receives Niri actions; Noctalia provides the settings panel, status display, and notification UI. Other compositors and shells are not supported integrations yet. The daemon can still be configured manually, and custom shell actions can invoke user commands, but this does not add native integration for another compositor or shell.

The daemon reads Linux evdev events directly, grabs the physical mouse, and forwards ordinary pointer events through a virtual uinput clone. It does not execute or depend on the old `mxmoused` program, Solaar, LogiOps, or logid.

## Features

- Hold the thumb button and move to dispatch directional actions; tap it to dispatch the tap action.
- Hold the thumb button and scroll the vertical wheel to dispatch scroll-up/down actions.
- Assign thumb + left, middle, and right click chords. Configured chords run on click-down and consume the click and release; unassigned chords pass through.
- Assign back (`BTN_SIDE`) and front (`BTN_EXTRA`) side-button actions. Unassigned side buttons pass through. The under-wheel button is not handled.
- Configure thumbwheel left/right actions independently for normal use and while holding the thumb button. Unassigned directions preserve horizontal scrolling.
- Choose Gesture or Meta-toggle mode per profile. Meta-toggle injects a supported modifier key on each thumb press and releases held virtual keys when the service exits.
- Assign profiles to exact Niri `app_id` values; all other apps use the configured default profile.
- Override DPI and vertical/horizontal wheel scales per profile. Profile changes follow focused-app changes without requiring a thumb-button press.
- Run Niri actions, Noctalia `msg` commands, or custom shell commands from configured actions.
- Scale standard and high-resolution vertical and horizontal wheel events independently.
- Set native HID++ adjustable DPI when the paired mouse supports it and the service user can access its receiver.

## Requirements and permissions

- Linux with evdev input access, a working `/dev/uinput`, Niri, and a user systemd manager.
- The running user needs read/write access to the mouse's `/dev/input/event*` node and `/dev/uinput`.
- DPI control additionally needs read/write access to the receiver's `/dev/hidraw*` node. DPI support and hidraw permissions vary by device and distribution.

On common distributions input nodes are owned by `root:input` with group read/write permission. Adding your user to `input` is one option; it grants access to all input devices, not just this mouse:

```sh
sudo usermod -aG input "$USER"
```

Fully log out and back in after changing group membership. If broader input access is not appropriate, use narrowly scoped udev permissions instead. `nnmxd` does not elevate privileges.

Check available devices and permissions with:

```sh
ls -l /dev/input/by-id /dev/uinput
```

## Installation

Install Git, Make, Go, and the Linux uinput kernel module, then clone and install:

```sh
git clone https://github.com/uheplm/nnmxd.git ~/.local/src/nnmxd
make -C ~/.local/src/nnmxd install
```

`make install` builds the binary into `~/.local/bin/nnmxd`, installs the `nnmxd.service` user unit, reloads systemd, and enables and starts the service. The build downloads Go modules. The [MX Mouse Noctalia plugin](https://github.com/uheplm/ffngs-plugins/tree/main/mxmouse) also offers this source-build installation as an optional wizard.

To rebuild and restart after changing the source:

```sh
make -C ~/.local/src/nnmxd reinstall
```

Useful service commands:

```sh
make -C ~/.local/src/nnmxd status
make -C ~/.local/src/nnmxd logs
make -C ~/.local/src/nnmxd stop
make -C ~/.local/src/nnmxd start
make -C ~/.local/src/nnmxd uninstall
```

If the legacy system-level `mxmoused.service` is installed, stop it before starting `nnmxd` so both daemons do not contend for the same device:

```sh
sudo systemctl disable --now mxmoused.service
```

## Service lifecycle

The user unit starts after `graphical-session.target`, runs `~/.local/bin/nnmxd`, and restarts the process after an unexpected failure. It does not run as root.

At startup the daemon loads and validates the configuration, then opens the configured mouse. With auto-detection enabled and no explicit device path, it scans evdev nodes for a Logitech vendor ID (`046d`), a mouse/receiver-like device name, and the configured thumb trigger key. If the config is invalid or no matching device is available, the process keeps running, writes the error to runtime status and the journal, and retries.

After opening the mouse, the daemon grabs it, creates a virtual pointer clone and a virtual keyboard for modifier injection, and starts processing input. Focused Niri app IDs are polled while connected. On shutdown it releases any injected modifier keys, removes the virtual devices, ungrabs the physical mouse, and clears runtime status.

Inspect service state and logs with:

```sh
systemctl --user status nnmxd.service --no-pager
journalctl --user -u nnmxd.service -f
```

## Configuration and applying changes

The default configuration file is `$XDG_CONFIG_HOME/mxmouse/config.json`, or `~/.config/mxmouse/config.json` when `XDG_CONFIG_HOME` is unset. When the file is absent, the service uses its built-in defaults. Configuration JSON is strict: invalid values, unknown fields, or an unsupported version are reported rather than silently accepted.

The daemon reads the configuration at startup and reloads it when the thumb button is pressed. Profile and action changes therefore take effect on the next press. The Noctalia plugin stages changes and calls:

```sh
nnmxd --apply-config /path/to/candidate-config.json
```

This command validates the candidate and atomically replaces the live configuration only if it is valid. The new configuration has an incremented `applyGeneration`. On the next thumb-button press, the daemon records the generation as applied and consumes that press and release; the configured gesture or modifier toggle is not run for that press. The applied generation is reported in runtime status. This confirmation step is how the panel knows the service has loaded the saved configuration.

Changing `device.autoDetect`, `device.path`, or `device.triggerButton` requires restarting the service because these values determine which physical device is opened and grabbed. DPI and profile DPI changes are queued for background application; per-profile DPI changes follow app focus. Scroll scales are used on subsequent wheel events.

The daemon is also usable without the panel: edit the JSON file, validate and atomically install it with `nnmxd --apply-config <candidate-file>`, then press the thumb button once to acknowledge and load it. Use `nnmxd --config <path>` to run against an alternate config file, and `nnmxd --version` to print the binary version.

## Configuration schema

The following is a complete example. Empty action `value` strings mean “unassigned”; the default profile values shown here can be changed to suit your setup.

```json
{
  "version": 1,
  "applyGeneration": 0,
  "device": {
    "autoDetect": true,
    "path": "",
    "triggerButton": "BTN_FORWARD",
    "dpi": 0,
    "verticalScroll": 100,
    "horizontalScroll": 100
  },
  "defaultPresetId": "default",
  "presets": [
    {
      "id": "default",
      "name": "Default",
      "mode": "gesture",
      "glyph": "arrows-move",
      "dpi": 0,
      "verticalScroll": 0,
      "horizontalScroll": 0,
      "threshold": 100,
      "retriggerDistance": 100,
      "escalation": 0,
      "metaKey": "KEY_LEFTMETA",
      "bindings": {
        "left": {"type": "niri", "value": "focus-column-left"},
        "right": {"type": "niri", "value": "focus-column-right"},
        "up": {"type": "niri", "value": "focus-workspace-up"},
        "down": {"type": "niri", "value": "focus-workspace-down"},
        "tap": {"type": "niri", "value": "toggle-overview"},
        "scrollUp": {"type": "niri", "value": "focus-workspace-up"},
        "scrollDown": {"type": "niri", "value": "focus-workspace-down"},
        "thumbwheelLeft": {"type": "niri", "value": ""},
        "thumbwheelRight": {"type": "niri", "value": ""},
        "thumbwheelButtonLeft": {"type": "niri", "value": ""},
        "thumbwheelButtonRight": {"type": "niri", "value": ""},
        "sideBack": {"type": "niri", "value": ""},
        "sideForward": {"type": "niri", "value": ""},
        "thumbLeftClick": {"type": "niri", "value": ""},
        "thumbMiddleClick": {"type": "niri", "value": ""},
        "thumbRightClick": {"type": "niri", "value": ""}
      }
    }
  ],
  "appPresets": {}
}
```

### Top-level fields

| Field | Meaning |
| --- | --- |
| `version` | Required schema version. Currently `1`. |
| `applyGeneration` | Non-negative integer used by the panel and service to acknowledge a saved configuration. Increment it when staging a panel-style apply. |
| `device` | Device selection, thumb trigger, default DPI, and default wheel scales. |
| `defaultPresetId` | ID of the fallback profile. Must match an entry in `presets`. |
| `presets` | One or more profile definitions with unique, non-empty IDs and names. |
| `appPresets` | Map of exact Niri `app_id` strings to profile IDs. Unlisted apps use the default profile. |

### Device fields

| Field | Meaning and valid values |
| --- | --- |
| `autoDetect` | Scan Logitech mouse/receiver-like evdev devices when `path` is empty. |
| `path` | Optional explicit `/dev/input/event*` path. When non-empty, this path is opened directly and takes precedence over auto-detection. When auto-detection is disabled, a path is required. |
| `triggerButton` | Linux evdev key name for the thumb trigger; defaults to `BTN_FORWARD`. It must be a key exposed by the selected device. |
| `dpi` | `0` leaves the observed hardware DPI unchanged; otherwise an integer from `100` to `16000`. The mouse must support the requested HID++ adjustable-DPI value. |
| `verticalScroll` | Vertical wheel multiplier as an integer percentage from `10` to `500`. Default `100`. |
| `horizontalScroll` | Horizontal wheel multiplier as an integer percentage from `10` to `500`. Default `100`. |

### Profile fields

| Field | Meaning and valid values |
| --- | --- |
| `id` | Unique, non-empty profile identifier. |
| `name` | Non-empty profile label. |
| `mode` | `gesture` or `meta-toggle`. |
| `glyph` | Optional Noctalia glyph name used by the panel. |
| `dpi` | `0` inherits `device.dpi`; otherwise an integer from `100` to `16000`. |
| `verticalScroll`, `horizontalScroll` | `0` inherits the matching device multiplier; otherwise an integer from `10` to `500`. |
| `threshold` | Positive evdev relative-motion count threshold for the first directional gesture. |
| `retriggerDistance` | Positive evdev motion count needed for each later action. Defaults to `threshold` when absent in an older configuration. |
| `escalation` | Non-negative count added to the retrigger distance after each directional action. `0` keeps later distances constant. |
| `metaKey` | Modifier injected in `meta-toggle` mode: `KEY_LEFTMETA`, `KEY_RIGHTMETA`, `KEY_LEFTCTRL`, `KEY_RIGHTCTRL`, `KEY_LEFTALT`, `KEY_RIGHTALT`, `KEY_LEFTSHIFT`, or `KEY_RIGHTSHIFT`. |
| `bindings` | Action definitions described below. |

`threshold`, `retriggerDistance`, `escalation`, and gesture bindings are used in Gesture mode. Side-button bindings work in either mode.

### Bindings and actions

| Binding | Input |
| --- | --- |
| `left`, `right`, `up`, `down` | Direction reached while moving with the thumb button held. |
| `tap` | Release of the thumb button without a gesture action. |
| `scrollUp`, `scrollDown` | Vertical wheel while the thumb button is held. |
| `thumbwheelLeft`, `thumbwheelRight` | Horizontal thumbwheel while the thumb button is not held. |
| `thumbwheelButtonLeft`, `thumbwheelButtonRight` | Horizontal thumbwheel while the thumb button is held. |
| `sideBack`, `sideForward` | Back (`BTN_SIDE`) and front (`BTN_EXTRA`) side buttons. |
| `thumbLeftClick`, `thumbMiddleClick`, `thumbRightClick` | Corresponding main mouse button pressed while a Gesture-mode thumb hold is active. |

An action is an object with `type`, `value`, and optional `arguments`:

| `type` | Behavior |
| --- | --- |
| `niri` | Sends the action through Niri IPC. `value` contains the Niri action and any arguments; `arguments`, when present, is appended. |
| `noctalia` | Runs `noctalia msg` with the arguments in `value`; `arguments`, when present, is appended. |
| `command` | Runs `value` as a shell command using `sh -c`. Custom commands run as the user and can execute arbitrary programs. |

Niri and Noctalia argument strings are split with simple shell-like quoting and escaping, but are not run through a shell. For example, `{"type":"noctalia","value":"panel-toggle","arguments":"ffngs/mxmouse:manager"}` opens the plugin panel. An empty `value` means the binding is unassigned. Older configurations may encode an action as a plain string; it is interpreted as a Niri action.

## How input is handled

1. The daemon opens the selected evdev device and grabs it so input events are routed through the daemon while it is active.
2. A uinput virtual pointer clone forwards ordinary pointer events. Wheel events are scaled according to the active profile and device settings.
3. While the thumb trigger is held in Gesture mode, relative X/Y movement is accumulated. Reversing direction clears the accumulation for that axis. Reaching the first threshold dispatches one directional action and resets movement; each later action requires `retriggerDistance + (actions already fired × escalation)` additional counts.
4. A tap action runs on release only if no gesture, scroll action, thumbwheel action, or configured thumb chord used the hold. Assigned chords run once on click-down and consume their matching release and repeats; unassigned clicks pass through as regular mouse clicks.
5. In Meta-toggle mode, each thumb press toggles the selected virtual modifier and forwards the physical button event. Held virtual modifier keys are released when the service exits.
6. Side-button actions are checked against the focused app's profile on each press. An assigned action consumes the press and release; unassigned buttons are forwarded.
7. Horizontal wheel events use separate bindings with and without the thumb hold. High-resolution horizontal deltas are accumulated into 120-unit detents, capped per event, and legacy horizontal events are skipped when the device also supplies high-resolution events.

Niri focus is polled approximately every 250 ms. Profile selection uses exact `app_id` matching, not window titles. The profile active when the thumb hold begins remains the gesture profile for that hold.

## DPI and scrolling

DPI uses the mouse's native HID++ adjustable-DPI feature over the receiver's hidraw device. No Solaar, LogiOps, or logid process is required. The service can capture the current hardware DPI as a baseline when device DPI is `0` and one or more profiles specify DPI. Profile-specific DPI is queued when focus changes and applied asynchronously. Transient errors are retried with increasing delays; unsupported DPI values are reported without retrying. Some mice may override host-set DPI through onboard profiles.

Vertical and horizontal wheel multipliers are applied independently to standard and high-resolution relative wheel events. `100` forwards original deltas unchanged; non-integral scaled deltas are carried forward as a remainder so low multipliers do not simply discard every small event. A profile multiplier of `0` inherits the corresponding device multiplier. Scales take effect on wheel events without restarting.

## Runtime status and troubleshooting

The service writes status to `$XDG_RUNTIME_DIR/mxmouse/status.json`; `XDG_RUNTIME_DIR` must be set by the user session. The JSON object contains:

| Field | Meaning |
| --- | --- |
| `running` | Whether an input device is currently attached and processed. |
| `device` | Attached device name, or empty when disconnected. |
| `mode` | Active profile mode. |
| `appId` | Focused Niri app ID. |
| `presetId`, `presetName` | Active profile identity. |
| `metaOn` | Whether a virtual modifier is currently latched. |
| `lastError` | Most recent config, device, input, or DPI error. |
| `appliedGeneration` | Highest configuration generation acknowledged by a thumb press. |
| `updatedUnix` | Status update time as Unix seconds. |

The status file is written atomically with user-only permissions. The plugin reads it for Overview status and the `modifier` bar widget. Detailed logs are available through `journalctl --user -u nnmxd.service`.

Common checks:

- Confirm Niri and the Noctalia shell are running; these integrations are the supported focus/action and panel/notification paths.
- Confirm the service user can read/write the selected `/dev/input/event*` node and `/dev/uinput`.
- For DPI issues, check permissions on the receiver's `/dev/hidraw*` node and whether the mouse supports the requested DPI.
- Check `device.path`, `device.autoDetect`, and `device.triggerButton`; restart the service after changing any of them.
- Stop any other daemon that has grabbed the same mouse.
- Review `lastError` in the status file and the user service journal.

## Development

```sh
make test
make build
```

The gesture engine and event-routing helpers have unit tests. Tests do not grab a physical mouse or inject uinput events.

## License

MIT
