package hidpp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseDPIValues(t *testing.T) {
	encoded := []byte{0x01, 0x90, 0x03, 0x20, 0xe0, 0x32, 0x1f, 0x40, 0, 0}
	got, err := parseDPIValues(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 146 || got[0] != 400 || got[1] != 800 || got[2] != 850 || got[len(got)-1] != 8000 {
		t.Fatalf("unexpected decoded DPI values: count=%d first=%v last=%v", len(got), got[:3], got[len(got)-1])
	}
}

func TestParseDPIValuesRejectsInvalidRanges(t *testing.T) {
	for name, encoded := range map[string][]byte{
		"missing start": {0xe0, 50, 0x03, 0x20, 0, 0},
		"zero step":     {0xe0, 0, 0x03, 0x20, 0, 0},
		"unterminated":  {0x01, 0x90},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseDPIValues(encoded); err == nil {
				t.Fatal("parseDPIValues succeeded; want an error")
			}
		})
	}
}

func TestBuildRequestUsesHIDPPReportOffsets(t *testing.T) {
	packet := buildRequest(2, 0x07, 0x30, 0x08, []byte{0x00, 0x03, 0xe8})
	if packet[0] != longReportID || packet[1] != 2 || packet[2] != 0x07 || packet[3] != 0x38 {
		t.Fatalf("HID++ header = %x", packet[:4])
	}
	if got := packet[4:7]; got[0] != 0 || got[1] != 0x03 || got[2] != 0xe8 {
		t.Fatalf("HID++ parameters = %x, want 0003e8", got)
	}
}

func TestDPIProbeFailureDoesNotClaimUnsupportedDevice(t *testing.T) {
	err := dpiDiscoveryError(errors.New("HID++ request timed out for device slot 6"))
	if err == nil || !strings.Contains(err.Error(), "could not confirm adjustable DPI support") {
		t.Fatalf("discovery error = %v, want an inconclusive support error", err)
	}
	if strings.Contains(err.Error(), "no paired Logitech device") {
		t.Fatalf("discovery error incorrectly claims unsupported DPI: %v", err)
	}
	if !strings.Contains(err.Error(), "retried on the next thumb-button press") {
		t.Fatalf("discovery error lacks retry guidance: %v", err)
	}
}

func TestDPIUnsupportedErrorRequiresResponsiveDevice(t *testing.T) {
	err := dpiDiscoveryError(nil)
	if err == nil || err.Error() != "no paired Logitech device on the selected receiver supports adjustable DPI" {
		t.Fatalf("discovery error = %v, want a confirmed unsupported error", err)
	}
}
