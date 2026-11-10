package auth

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestCheckWithoutPolkit(t *testing.T) {
	uids := map[dbus.Sender]uint32{":1.1": 0, ":1.2": 1000, ":1.3": 1001}
	a := &Authorizer{
		Caller: func(s dbus.Sender) (uint32, error) {
			uid, ok := uids[s]
			if !ok {
				return 0, errors.New("gone")
			}
			return uid, nil
		},
		IsAdmin: func(uid uint32) bool { return uid == 1000 },
	}
	if e := a.Check(":1.1", "x"); e != nil {
		t.Fatalf("root refused: %v", e)
	}
	if e := a.Check(":1.2", "x"); e != nil {
		t.Fatalf("admin refused: %v", e)
	}
	if e := a.Check(":1.3", "x"); e == nil || e.Name != "org.freedesktop.DBus.Error.AccessDenied" {
		t.Fatalf("plain user = %v", e)
	}
	if e := a.Check(":1.4", "x"); e == nil {
		t.Fatalf("unknown caller allowed")
	}
}
