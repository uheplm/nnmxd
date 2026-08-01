package logid

import (
	"errors"
	"fmt"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

func TestDecodeNameOwnerChanged(t *testing.T) {
	cases := []struct {
		name     string
		body     []any
		wantName string
		wantOld  string
		wantNew  string
		wantOK   bool
	}{
		{
			name:     "up transition",
			body:     []any{"pizza.pixl.LogiOps", "", ":1.42"},
			wantName: "pizza.pixl.LogiOps",
			wantNew:  ":1.42",
			wantOK:   true,
		},
		{
			name:     "down transition",
			body:     []any{"pizza.pixl.LogiOps", ":1.42", ""},
			wantName: "pizza.pixl.LogiOps",
			wantOld:  ":1.42",
			wantOK:   true,
		},
		{
			name: "short body",
			body: []any{"pizza.pixl.LogiOps", ""},
		},
		{
			name: "long body",
			body: []any{"a", "b", "c", "d"},
		},
		{
			name: "non-string field",
			body: []any{"a", 42, "c"},
		},
		{
			name: "nil body",
			body: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, oldOwner, newOwner, ok := decodeNameOwnerChanged(tc.body)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if name != tc.wantName || oldOwner != tc.wantOld || newOwner != tc.wantNew {
				t.Errorf("got (%q,%q,%q), want (%q,%q,%q)",
					name, oldOwner, newOwner,
					tc.wantName, tc.wantOld, tc.wantNew)
			}
		})
	}
}

func TestParseDeviceChildren(t *testing.T) {
	cases := []struct {
		name   string
		parent dbus.ObjectPath
		node   *introspect.Node
		want   []dbus.ObjectPath
	}{
		{
			name:   "nil node",
			parent: devicesPath,
			node:   nil,
			want:   nil,
		},
		{
			name:   "no children",
			parent: devicesPath,
			node:   &introspect.Node{},
			want:   []dbus.ObjectPath{},
		},
		{
			name:   "two children",
			parent: devicesPath,
			node: &introspect.Node{
				Children: []introspect.Node{
					{Name: "0"},
					{Name: "1"},
				},
			},
			want: []dbus.ObjectPath{
				"/pizza/pixl/logiops/devices/0",
				"/pizza/pixl/logiops/devices/1",
			},
		},
		{
			name:   "skips empty-name child",
			parent: devicesPath,
			node: &introspect.Node{
				Children: []introspect.Node{
					{Name: ""},
					{Name: "0"},
				},
			},
			want: []dbus.ObjectPath{"/pizza/pixl/logiops/devices/0"},
		},
		{
			name:   "root parent",
			parent: dbus.ObjectPath("/"),
			node:   &introspect.Node{Children: []introspect.Node{{Name: "foo"}}},
			want:   []dbus.ObjectPath{"/foo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDeviceChildren(tc.parent, tc.node)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestIsNoObjectError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("random"), want: false},
		{
			name: "unknown object",
			err:  &dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownObject"},
			want: true,
		},
		{
			name: "service unknown",
			err:  &dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"},
			want: true,
		},
		{
			name: "unknown interface",
			err:  &dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownInterface"},
			want: true,
		},
		{
			name: "unknown method",
			err:  &dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownMethod"},
			want: true,
		},
		{
			name: "wrapped unknown object",
			err: fmt.Errorf("introspect: %w",
				&dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownObject"}),
			want: true,
		},
		{
			name: "other dbus error",
			err:  &dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoObjectError(tc.err); got != tc.want {
				t.Errorf("isNoObjectError = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindDeviceUnavailable(t *testing.T) {
	// Zero-valued Client: available atomic.Bool defaults to false, which is
	// exactly the state we want. No bus is opened.
	c := &Client{}
	_, err := c.FindDevice("anything")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("FindDevice err = %v, want ErrUnavailable", err)
	}
}
