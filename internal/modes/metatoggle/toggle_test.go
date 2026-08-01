package metatoggle

import (
	"encoding/json"
	"testing"
)

func TestBuildStateOn(t *testing.T) {
	s := BuildState(true)
	if s.Text != "META" || s.Alt != "on" || s.Class != "on" {
		t.Errorf("on state wrong: %+v", s)
	}
}

func TestBuildStateOff(t *testing.T) {
	s := BuildState(false)
	if s.Text != "" || s.Alt != "off" || s.Class != "off" {
		t.Errorf("off state wrong: %+v", s)
	}
}

func TestBuildStateJSONShapeMatchesV1(t *testing.T) {
	// The Waybar module expects exactly these four keys, in any order.
	b, err := json.Marshal(BuildState(true))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"text", "alt", "class", "tooltip"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
}
