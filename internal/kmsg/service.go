package kmsg

import (
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	Name   = "dev.sinty.Logs1"
	Path   = dbus.ObjectPath("/dev/sinty/Logs1")
	Iface  = "dev.sinty.Logs1"
	Action = "dev.sinty.logs.read-kernel"
)

type Checker interface {
	Check(sender dbus.Sender, action string) *dbus.Error
}

type Service struct {
	device string
	auth   Checker
}

func NewService(device string, auth Checker) *Service {
	return &Service{device: device, auth: auth}
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

func (s *Service) ReadKernel(sender dbus.Sender) (string, *dbus.Error) {
	if e := s.auth.Check(sender, Action); e != nil {
		return "", e
	}
	records, err := Read(s.device)
	if err != nil && len(records) == 0 {
		return "", dbus.NewError(Iface+".Error.Failed", []interface{}{err.Error()})
	}
	return Format(records), nil
}
