package hidpp

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	shortReportID = 0x10
	longReportID  = 0x11
	reportSize    = 20
	featureSet    = 0x0001
	adjustableDPI = 0x2201
	maxDPI        = 16000
)

type client struct {
	fd     int
	device byte
	swID   byte
}

func SetDPI(inputPath string, dpi int) error {
	if dpi < 100 || dpi > maxDPI {
		return fmt.Errorf("DPI %d is outside the supported range 100-%d", dpi, maxDPI)
	}
	hidraw, err := findHIDPPNode(inputPath)
	if err != nil {
		return err
	}
	fd, err := syscall.Open(hidraw, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open Logitech HID++ interface %s: %w", hidraw, err)
	}
	defer syscall.Close(fd)

	var lastDiscoveryError error
	for deviceNumber := byte(1); deviceNumber <= 6; deviceNumber++ {
		c := &client{fd: fd, device: deviceNumber, swID: 0x08}
		featureIndex, _, err := c.findFeature(adjustableDPI)
		if err != nil {
			lastDiscoveryError = err
			continue
		}
		if featureIndex == 0 {
			continue
		}
		values, err := c.dpiValues(featureIndex)
		if err != nil {
			return fmt.Errorf("read supported DPI values from device slot %d: %w", deviceNumber, err)
		}
		if !contains(values, dpi) {
			return fmt.Errorf("DPI %d is not supported by device slot %d (supported values: %s)", dpi, deviceNumber, formatValues(values))
		}
		if _, err := c.request(featureIndex, 0x30, 0x00, byte(dpi>>8), byte(dpi)); err != nil {
			return fmt.Errorf("set DPI on device slot %d: %w", deviceNumber, err)
		}
		current, err := c.request(featureIndex, 0x20)
		if err != nil {
			return fmt.Errorf("read back DPI from device slot %d: %w", deviceNumber, err)
		}
		if len(current) < 5 {
			return fmt.Errorf("read back DPI from device slot %d: response was incomplete", deviceNumber)
		}
		actual := int(current[1])<<8 | int(current[2])
		if actual == 0 {
			actual = int(current[3])<<8 | int(current[4])
		}
		if actual != dpi {
			return fmt.Errorf("device slot %d reports DPI %d after requesting %d", deviceNumber, actual, dpi)
		}
		return nil
	}
	return dpiDiscoveryError(lastDiscoveryError)
}

func dpiDiscoveryError(lastDiscoveryError error) error {
	if lastDiscoveryError != nil {
		return fmt.Errorf("could not confirm adjustable DPI support; it will be retried on the next thumb-button press: %w", lastDiscoveryError)
	}
	return errors.New("no paired Logitech device on the selected receiver supports adjustable DPI")
}

func (c *client) findFeature(featureID uint16) (byte, bool, error) {
	root, err := c.request(0, 0, byte(featureSet>>8), byte(featureSet))
	if err != nil || len(root) < 1 || root[0] == 0 {
		if err == nil {
			err = errors.New("HID++ feature set response was incomplete")
		}
		return 0, false, err
	}
	featureSetIndex := root[0]
	count, err := c.request(featureSetIndex, 0)
	if err != nil || len(count) < 1 {
		if err == nil {
			err = errors.New("HID++ feature count response was incomplete")
		}
		return 0, false, err
	}
	var lastLookupError error
	for index := 1; index <= int(count[0]); index++ {
		entry, err := c.request(featureSetIndex, 0x10, byte(index))
		if err != nil {
			lastLookupError = err
			continue
		}
		if len(entry) >= 2 && uint16(entry[0])<<8|uint16(entry[1]) == featureID {
			return byte(index), true, nil
		}
		if len(entry) < 2 {
			lastLookupError = errors.New("HID++ feature lookup response was incomplete")
		}
	}
	if lastLookupError != nil {
		return 0, false, lastLookupError
	}
	return 0, true, nil
}

func (c *client) dpiValues(featureIndex byte) ([]int, error) {
	var encoded []byte
	for page := byte(0); page < 0xff; page++ {
		reply, err := c.request(featureIndex, 0x10, 0x00, 0x00, page)
		if err != nil {
			return nil, err
		}
		if len(reply) < 3 {
			return nil, errors.New("DPI list response was incomplete")
		}
		encoded = append(encoded, reply[1:]...)
		if bytes.HasSuffix(encoded, []byte{0, 0}) {
			break
		}
	}
	values, err := parseDPIValues(encoded)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, errors.New("device returned an empty DPI list")
	}
	return values, nil
}

func parseDPIValues(encoded []byte) ([]int, error) {
	var values []int
	for offset := 0; offset+1 < len(encoded); {
		value := int(encoded[offset])<<8 | int(encoded[offset+1])
		if value == 0 {
			return values, nil
		}
		if value>>13 == 0b111 {
			step := value & 0x1fff
			if step == 0 || offset+3 >= len(encoded) || len(values) == 0 {
				return nil, errors.New("device returned an invalid DPI range")
			}
			last := int(encoded[offset+2])<<8 | int(encoded[offset+3])
			if last <= values[len(values)-1] {
				return nil, errors.New("device returned a non-increasing DPI range")
			}
			for next := values[len(values)-1] + step; next <= last; next += step {
				values = append(values, next)
			}
			offset += 4
			continue
		}
		values = append(values, value)
		offset += 2
	}
	return nil, errors.New("device returned an unterminated DPI list")
}

func (c *client) request(feature, function byte, params ...byte) ([]byte, error) {
	if len(params) > reportSize-4 {
		return nil, errors.New("HID++ request has too many parameters")
	}
	packet := buildRequest(c.device, feature, function, c.swID, params)
	if n, err := syscall.Write(c.fd, packet[:]); err != nil {
		return nil, fmt.Errorf("write HID++ request: %w", err)
	} else if n != len(packet) {
		return nil, fmt.Errorf("write HID++ request: short write of %d bytes", n)
	}

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		var reply [reportSize]byte
		n, err := syscall.Read(c.fd, reply[:])
		if err != nil && err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("read HID++ response: %w", err)
		}
		if err == nil && n >= 5 && (reply[0] == shortReportID || reply[0] == longReportID) && reply[1] == c.device {
			if n >= 6 && reply[2] == 0xff && reply[3] == feature && reply[4] == packet[3] {
				return nil, fmt.Errorf("HID++ device rejected request with error 0x%02x", reply[5])
			}
			if reply[2] == feature && reply[3] == packet[3] {
				return reply[4:n], nil
			}
		}
		time.Sleep(time.Millisecond)
	}
	return nil, fmt.Errorf("HID++ request timed out for device slot %d", c.device)
}

func buildRequest(device, feature, function, swID byte, params []byte) [reportSize]byte {
	var packet [reportSize]byte
	packet[0] = longReportID
	packet[1] = device
	packet[2] = feature
	packet[3] = function | swID
	copy(packet[4:], params)
	return packet
}

func findHIDPPNode(inputPath string) (string, error) {
	resolvedInput, err := filepath.EvalSymlinks(inputPath)
	if err != nil {
		return "", fmt.Errorf("resolve input device %s: %w", inputPath, err)
	}
	eventName := filepath.Base(resolvedInput)
	inputSysfs, err := filepath.EvalSymlinks(filepath.Join("/sys/class/input", eventName, "device"))
	if err != nil {
		return "", fmt.Errorf("resolve input device sysfs path: %w", err)
	}
	receiverUSB, err := usbDevicePath(inputSysfs)
	if err != nil {
		return "", fmt.Errorf("locate input device's USB receiver: %w", err)
	}
	entries, err := os.ReadDir("/sys/class/hidraw")
	if err != nil {
		return "", fmt.Errorf("list HID++ interfaces: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		deviceSysfs, err := filepath.EvalSymlinks(filepath.Join("/sys/class/hidraw", name, "device"))
		if err != nil {
			continue
		}
		candidateUSB, err := usbDevicePath(deviceSysfs)
		if err != nil || candidateUSB != receiverUSB {
			continue
		}
		descriptor, err := os.ReadFile(filepath.Join(deviceSysfs, "report_descriptor"))
		if err != nil || !hasHIDPPReports(descriptor) {
			continue
		}
		return filepath.Join("/dev", name), nil
	}
	return "", errors.New("no HID++ interface found on the selected Logitech USB receiver")
}

func usbDevicePath(path string) (string, error) {
	for current := path; current != filepath.Dir(current); current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "idVendor")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "idProduct")); err == nil {
				return current, nil
			}
		}
	}
	return "", errors.New("USB device identity is unavailable")
}

func hasHIDPPReports(descriptor []byte) bool {
	return bytes.Contains(descriptor, []byte{0x85, shortReportID}) &&
		bytes.Contains(descriptor, []byte{0x85, longReportID})
}

func contains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func formatValues(values []int) string {
	var builder strings.Builder
	for index, value := range values {
		if index > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprint(&builder, value)
	}
	return builder.String()
}
