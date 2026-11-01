package service

import (
	"reflect"
	"testing"

	"github.com/uheplm/nnmxd/internal/config"
)

func TestSplitActionArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "plain args", input: `audio-volume-set 45`, want: []string{"audio-volume-set", "45"}},
		{name: "quoted arg", input: `notification-show "Hello there"`, want: []string{"notification-show", "Hello there"}},
		{name: "escaped space", input: `spawn-sh notify-send\ "Hello"`, want: []string{"spawn-sh", "notify-send Hello"}},
		{name: "empty", input: " \t ", want: nil},
		{name: "unclosed quote", input: `panel-toggle "ffngs/mxmouse`, wantErr: true},
		{name: "trailing escape", input: `audio-volume-set\`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := splitActionArguments(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("splitActionArguments() error = %v, wantErr %v", err, test.wantErr)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("splitActionArguments() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestActionArguments(t *testing.T) {
	tests := []struct {
		name string
		in   config.Action
		want []string
	}{
		{
			name: "selected command with parameters",
			in: config.Action{
				Type: config.ActionNoctalia, Value: "panel-toggle",
				Arguments: `"ffngs/mxmouse:manager"`,
			},
			want: []string{"panel-toggle", "ffngs/mxmouse:manager"},
		},
		{
			name: "legacy value containing parameters",
			in:   config.Action{Type: config.ActionNiri, Value: `focus-column 2`},
			want: []string{"focus-column", "2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := actionArguments(test.in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("actionArguments() = %#v, want %#v", got, test.want)
			}
		})
	}
}
