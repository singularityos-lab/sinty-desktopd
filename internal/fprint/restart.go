package fprint

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/godbus/dbus/v5"
)

func Trigger(sysfs, id string) int {
	vendor, product, ok := strings.Cut(id, ":")
	if !ok {
		return 0
	}
	dirs, _ := filepath.Glob(filepath.Join(sysfs, "bus", "usb", "devices", "*"))
	count := 0
	for _, dir := range dirs {
		v, err1 := os.ReadFile(filepath.Join(dir, "idVendor"))
		p, err2 := os.ReadFile(filepath.Join(dir, "idProduct"))
		if err1 != nil || err2 != nil {
			continue
		}
		if strings.TrimSpace(string(v)) != vendor || strings.TrimSpace(string(p)) != product {
			continue
		}
		if os.WriteFile(filepath.Join(dir, "uevent"), []byte("add"), 0) == nil {
			count++
		}
	}
	return count
}

func FprintdPID(conn *dbus.Conn, proc string) int {
	var pid uint32
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, "net.reactivated.Fprint").Store(&pid); err != nil || pid <= 1 {
		return 0
	}
	exe, err := os.Readlink(filepath.Join(proc, strconv.FormatUint(uint64(pid), 10), "exe"))
	if err != nil {
		return 0
	}
	if filepath.Base(strings.TrimSuffix(exe, " (deleted)")) != "fprintd" {
		return 0
	}
	return int(pid)
}

func Restarter(conn *dbus.Conn) func(id string) {
	return func(id string) {
		Trigger("/sys", id)
		if pid := FprintdPID(conn, "/proc"); pid > 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	}
}
