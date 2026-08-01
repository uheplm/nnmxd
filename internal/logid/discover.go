package logid

import (
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// parseDeviceChildren turns an introspection result into full object paths
// for every named child. Pure function — no bus contact.
func parseDeviceChildren(parent dbus.ObjectPath, node *introspect.Node) []dbus.ObjectPath {
	if node == nil {
		return nil
	}
	out := make([]dbus.ObjectPath, 0, len(node.Children))
	base := string(parent)
	if base == "/" {
		base = ""
	}
	for _, child := range node.Children {
		if child.Name == "" {
			continue
		}
		out = append(out, dbus.ObjectPath(base+"/"+child.Name))
	}
	return out
}

// decodeNameOwnerChanged decodes the body of an org.freedesktop.DBus
// NameOwnerChanged signal into its (name, oldOwner, newOwner) triplet.
// Returns ok=false for malformed bodies.
func decodeNameOwnerChanged(body []any) (name, oldOwner, newOwner string, ok bool) {
	if len(body) != 3 {
		return "", "", "", false
	}
	n, ok1 := body[0].(string)
	o, ok2 := body[1].(string)
	nw, ok3 := body[2].(string)
	if !ok1 || !ok2 || !ok3 {
		return "", "", "", false
	}
	return n, o, nw, true
}
