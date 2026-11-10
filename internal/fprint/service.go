package fprint

import (
	"errors"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	Name   = "dev.sinty.FingerprintDriver1"
	Path   = dbus.ObjectPath("/dev/sinty/FingerprintDriver1")
	Iface  = "dev.sinty.FingerprintDriver1"
	Action = "dev.sinty.fprint-driver"
)

type Checker interface {
	Check(sender dbus.Sender, action string) *dbus.Error
}

type Service struct {
	mu        sync.Mutex
	installer *Installer
	auth      Checker
}

func NewService(installer *Installer, auth Checker) *Service {
	return &Service{installer: installer, auth: auth}
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
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalid):
		return dbus.NewError(Iface+".Error.Invalid", []interface{}{err.Error()})
	case errors.Is(err, ErrNotFound):
		return dbus.NewError(Iface+".Error.NotFound", []interface{}{err.Error()})
	}
	return dbus.NewError(Iface+".Error.Failed", []interface{}{err.Error()})
}

func (s *Service) Info(id string) (map[string]string, *dbus.Error) {
	info, err := s.installer.Info(id)
	return info, reply(err)
}

func (s *Service) Install(sender dbus.Sender, id string) *dbus.Error {
	if _, err := s.installer.Lookup(id); err != nil {
		return reply(err)
	}
	if e := s.auth.Check(sender, Action); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return reply(s.installer.Install(id))
}

func (s *Service) Uninstall(sender dbus.Sender, id string) *dbus.Error {
	if _, err := s.installer.Lookup(id); err != nil {
		return reply(err)
	}
	if e := s.auth.Check(sender, Action); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return reply(s.installer.Uninstall(id))
}
