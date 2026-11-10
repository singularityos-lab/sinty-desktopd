package loginscreen

import (
	"errors"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	Name   = "dev.sinty.LoginScreen1"
	Path   = dbus.ObjectPath("/dev/sinty/LoginScreen1")
	Iface  = "dev.sinty.LoginScreen1"
	Action = "dev.sinty.login-screen"
)

type Checker interface {
	Check(sender dbus.Sender, action string) *dbus.Error
}

type Service struct {
	mu    sync.Mutex
	store Store
	auth  Checker
}

func NewService(store Store, auth Checker) *Service {
	return &Service{store: store, auth: auth}
}

func (s *Service) Export(conn *dbus.Conn) error {
	if err := conn.Export(s, Path, Iface); err != nil {
		return err
	}
	node := &introspect.Node{
		Name: string(Path),
		Interfaces: []introspect.Interface{introspect.IntrospectData, {
			Name:    Iface,
			Methods: introspect.Methods(s),
		}},
	}
	return conn.Export(introspect.NewIntrospectable(node), Path, "org.freedesktop.DBus.Introspectable")
}

func reply(err error) *dbus.Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalid) {
		return dbus.NewError(Iface+".Error.Invalid", []interface{}{err.Error()})
	}
	return dbus.NewError(Iface+".Error.Failed", []interface{}{err.Error()})
}

func (s *Service) run(sender dbus.Sender, op func() error) *dbus.Error {
	if e := s.auth.Check(sender, Action); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return reply(op())
}

func (s *Service) Apply(sender dbus.Sender, payload string) *dbus.Error {
	return s.run(sender, func() error { return s.store.Apply(payload) })
}

func (s *Service) Reset(sender dbus.Sender) *dbus.Error {
	return s.run(sender, s.store.Reset)
}

func (s *Service) SetBackgroundImage(sender dbus.Sender, data []byte) *dbus.Error {
	return s.run(sender, func() error { return s.store.SetBackgroundImage(data) })
}

func (s *Service) SetBackgroundColor(sender dbus.Sender, color string) *dbus.Error {
	return s.run(sender, func() error { return s.store.SetBackgroundColor(color) })
}

func (s *Service) ResetBackground(sender dbus.Sender) *dbus.Error {
	return s.run(sender, s.store.ResetBackground)
}
