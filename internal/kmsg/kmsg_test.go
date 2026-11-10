package kmsg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParse(t *testing.T) {
	r, ok := Parse("6,1042,5123456,-;usb 1-2: new device\\x0aline two\n SUBSYSTEM=usb\n DEVICE=c189:1\n")
	if !ok || r.Priority != 6 || r.Usec != 5123456 || r.Message != "usb 1-2: new device\nline two" {
		t.Fatalf("Parse = %+v, %v", r, ok)
	}
	r, ok = Parse("30,7,42,c;tab\\x09bell\\x07end")
	if !ok || r.Priority != 30 || r.Message != "tab\tbell end" {
		t.Fatalf("Parse = %+v", r)
	}
	for _, bad := range []string{"", "no separator", "x,1,2;m", "6,1;m", "6,1,y;m"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestFormat(t *testing.T) {
	got := Format([]Record{{3, 1500000, "boom"}, {6, 12, "a\nb"}})
	want := "<3>[    1.500000] boom\n<6>[    0.000012] a\n<6>[    0.000012] b\n"
	if got != want {
		t.Fatalf("Format = %q", got)
	}
}

func TestReadMissing(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "nope")); !os.IsNotExist(err) {
		t.Fatalf("Read missing = %v", err)
	}
}

type deny struct{}

func (deny) Check(dbus.Sender, string) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{"no"})
}

type allow struct{}

func (allow) Check(dbus.Sender, string) *dbus.Error { return nil }

func TestServiceAuthorization(t *testing.T) {
	file := filepath.Join(t.TempDir(), "kmsg")
	os.WriteFile(file, []byte("4,1,2000000,-;secret\n"), 0o644)
	if out, e := NewService(file, deny{}).ReadKernel(":1.3"); e == nil || out != "" {
		t.Fatalf("denied read returned %q, %v", out, e)
	}
	out, e := NewService(file, allow{}).ReadKernel(":1.3")
	if e != nil || out != "<4>[    2.000000] secret\n" {
		t.Fatalf("ReadKernel = %q, %v", out, e)
	}
	if _, e := NewService(file+".none", allow{}).ReadKernel(":1.3"); e == nil || e.Name != Iface+".Error.Failed" {
		t.Fatalf("missing device = %v", e)
	}
}
