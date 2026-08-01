package logid

import (
	"errors"

	"github.com/godbus/dbus/v5"
)

// ErrUnavailable is returned by Client and Device methods when logid is not
// currently reachable on the system bus. Callers can wait for PresenceUp on
// Client.Watch before retrying.
var ErrUnavailable = errors.New("logid: not available")

// isNoObjectError reports whether err represents a D-Bus call against a
// path, service, interface, or method that no longer exists — typically
// because logid was restarted between discovery and the call.
func isNoObjectError(err error) bool {
	if err == nil {
		return false
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) || dbusErr == nil {
		return false
	}
	switch dbusErr.Name {
	case "org.freedesktop.DBus.Error.UnknownObject",
		"org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.UnknownInterface",
		"org.freedesktop.DBus.Error.UnknownMethod":
		return true
	}
	return false
}
