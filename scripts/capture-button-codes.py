#!/usr/bin/env python3
"""Prompt for mouse buttons and report their Linux evdev codes.

nnmxd exclusively grabs its input device, so stop the service before running
this script. The script only reads input events; it does not change settings.
"""

import argparse
import fcntl
import glob
import os
import re
import select
import struct
import sys
import time
from pathlib import Path

INPUT_EVENT = struct.Struct("@llHHI")
CAPABILITY_WORD_BITS = struct.calcsize("@L") * 8
EV_KEY = 1
BTN_MOUSE = 0x110
PROMPTS = ("First side button", "Second side button", "Button under the wheel")


def button_names():
    names = {}
    headers = (
        Path("/usr/include/linux/input-event-codes.h"),
        Path("/usr/include/linux/input.h"),
    )
    pattern = re.compile(r"^\s*#define\s+(BTN_[A-Z0-9_]+)\s+(0x[0-9a-fA-F]+|[0-9]+)")
    for header in headers:
        try:
            lines = header.read_text(encoding="ascii").splitlines()
        except OSError:
            continue
        for line in lines:
            match = pattern.match(line)
            if match:
                names[int(match.group(2), 0)] = match.group(1)
        if names:
            break
    return names


def advertises_mouse_button(event_path):
    capability_path = Path("/sys/class/input") / Path(event_path).name / "device/capabilities/key"
    try:
        words = [int(word, 16) for word in capability_path.read_text().split()]
    except (OSError, ValueError):
        return False
    word_index = len(words) - 1 - BTN_MOUSE // CAPABILITY_WORD_BITS
    return word_index >= 0 and bool(
        words[word_index] & (1 << (BTN_MOUSE % CAPABILITY_WORD_BITS))
    )


def device_name(event_path):
    name_path = Path("/sys/class/input") / Path(event_path).name / "device/name"
    try:
        return name_path.read_text(encoding="utf-8").strip()
    except OSError:
        return "Unknown input device"


def list_devices():
    return [
        (path, device_name(path))
        for path in sorted(glob.glob("/dev/input/event*"))
        if advertises_mouse_button(path)
    ]


def choose_device(devices):
    print("Mouse input devices:")
    for index, (path, name) in enumerate(devices, start=1):
        print(f"  {index}. {path} — {name}")
    while True:
        answer = input("Select a device number: ").strip()
        try:
            index = int(answer)
        except ValueError:
            print("Enter one of the listed numbers.")
            continue
        if 1 <= index <= len(devices):
            return devices[index - 1][0]
        print("Enter one of the listed numbers.")


def read_keydown(fd, timeout):
    deadline = time.monotonic() + timeout
    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return None
        readable, _, _ = select.select([fd], [], [], remaining)
        if not readable:
            return None
        data = os.read(fd, INPUT_EVENT.size * 32)
        for offset in range(0, len(data) - INPUT_EVENT.size + 1, INPUT_EVENT.size):
            _, _, event_type, code, value = INPUT_EVENT.unpack_from(data, offset)
            if event_type == EV_KEY and value == 1:
                return code


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--device",
        help="input event device (for example, /dev/input/event11); prompts if omitted",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=20,
        help="seconds to wait for each button press (default: 20)",
    )
    args = parser.parse_args()
    if args.timeout <= 0:
        parser.error("--timeout must be greater than zero")

    device = args.device
    if device is None:
        devices = list_devices()
        if not devices:
            print("No accessible mouse input devices were found under /dev/input.", file=sys.stderr)
            return 1
        device = choose_device(devices)

    names = button_names()
    try:
        fd = os.open(device, os.O_RDONLY | os.O_NONBLOCK)
    except OSError as error:
        print(f"Cannot open {device}: {error}", file=sys.stderr)
        return 1

    results = []
    try:
        print(f"\nListening on {device} ({device_name(device)}).")
        print("Press each requested control once; press Ctrl+C to stop.\n")
        for prompt in PROMPTS:
            input(f"{prompt}: press Enter when ready, then press the control. ")
            code = read_keydown(fd, args.timeout)
            if code is None:
                print(f"No button press detected within {args.timeout:g} seconds.")
                print("Check the selected device and ensure nnmxd is stopped, then rerun.")
                continue
            name = names.get(code, "unknown button")
            results.append((prompt, code, name))
            print(f"  Detected {name} (code {code}, 0x{code:03x})")
    except KeyboardInterrupt:
        print("\nCapture stopped.")
    except OSError as error:
        print(f"Could not read input events from {device}: {error}", file=sys.stderr)
        return 1
    finally:
        os.close(fd)

    if results:
        print("\nCaptured mappings:")
        for prompt, code, name in results:
            print(f"  {prompt}: {name} (code {code}, 0x{code:03x})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
