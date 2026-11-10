package auth

import (
	"os/user"
	"strconv"

	"github.com/godbus/dbus/v5"
)

const polkitName = "org.freedesktop.PolicyKit1"

var DefaultAdminGroups = []string{"wheel", "sudo", "admin"}

type Authorizer struct {
	Conn        *dbus.Conn
	AdminGroups []string
	Caller      func(sender dbus.Sender) (uint32, error)
	IsAdmin     func(uid uint32) bool
}

func Denied(message string) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{message})
}

func (a *Authorizer) caller(sender dbus.Sender) (uint32, error) {
	if a.Caller != nil {
		return a.Caller(sender)
	}
	var uid uint32
	err := a.Conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixUser", 0, string(sender)).Store(&uid)
	return uid, err
}

func (a *Authorizer) Check(sender dbus.Sender, action string) *dbus.Error {
	uid, err := a.caller(sender)
	if err != nil {
		return Denied("the caller could not be identified")
	}
	if uid == 0 {
		return nil
	}
	if a.Conn != nil && a.hasPolkit() {
		subject := struct {
			Kind    string
			Details map[string]dbus.Variant
		}{"system-bus-name", map[string]dbus.Variant{"name": dbus.MakeVariant(string(sender))}}
		var res struct {
			Authorized bool
			Challenge  bool
			Details    map[string]string
		}
		obj := a.Conn.Object(polkitName, "/org/freedesktop/PolicyKit1/Authority")
		call := obj.Call("org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0,
			subject, action, map[string]string{}, uint32(1), "")
		if call.Err == nil && call.Store(&res) == nil {
			if res.Authorized {
				return nil
			}
			return Denied("not authorized")
		}
	}
	if a.isAdmin(uid) {
		return nil
	}
	return Denied("not authorized")
}

func (a *Authorizer) hasPolkit() bool {
	var owned bool
	if err := a.Conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, polkitName).Store(&owned); err == nil && owned {
		return true
	}
	var names []string
	if err := a.Conn.BusObject().Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&names); err != nil {
		return false
	}
	for _, n := range names {
		if n == polkitName {
			return true
		}
	}
	return false
}

func (a *Authorizer) isAdmin(uid uint32) bool {
	if a.IsAdmin != nil {
		return a.IsAdmin(uid)
	}
	groups := a.AdminGroups
	if groups == nil {
		groups = DefaultAdminGroups
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return false
	}
	ids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, id := range ids {
		g, err := user.LookupGroupId(id)
		if err != nil {
			continue
		}
		for _, want := range groups {
			if g.Name == want {
				return true
			}
		}
	}
	return false
}
