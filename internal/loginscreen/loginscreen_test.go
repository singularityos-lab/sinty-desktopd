package loginscreen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
)

const payload = "%%FILE rc.xml\n<?xml version=\"1.0\"?>\n<labwc_config>\n</labwc_config>\n" +
	"%%FILE output.xml\n<outputs/>\n" +
	"%%FILE environment\nXCURSOR_THEME=Singularity\nXKB_DEFAULT_LAYOUT=it\n"

func TestParse(t *testing.T) {
	files, err := Parse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files["output.xml"] != "<outputs/>\n" || files["environment"] != "XCURSOR_THEME=Singularity\nXKB_DEFAULT_LAYOUT=it\n" {
		t.Fatalf("files = %q", files)
	}
	bad := []string{
		"stray\n%%FILE rc.xml\n<a/>\n",
		"%%FILE ../passwd\nroot\n",
		"%%FILE rc.xml\n<keybind><action name=\"Execute\"/></keybind>\n",
		"%%FILE rc.xml\nplain\n",
		"%%FILE environment\nLD_PRELOAD=/tmp/x.so\n",
		"%%FILE environment\nXCURSOR_THEME=$(id)\n",
	}
	for _, p := range bad {
		if _, err := Parse(p); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want invalid", p, err)
		}
	}
	big := make([]byte, MaxPayload+1)
	if _, err := Parse(string(big)); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversized payload accepted")
	}
}

func TestApplyAndReset(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Apply(payload); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.Dir, "labwc", "output.xml"))
	if err != nil || string(got) != "<outputs/>\n" {
		t.Fatalf("output.xml = %q, %v", got, err)
	}
	if err := s.Apply("%%FILE output.xml\n<outputs2/>\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "labwc", "rc.xml")); !os.IsNotExist(err) {
		t.Fatalf("stale rc.xml survived a new apply")
	}
	if err := s.Apply("%%FILE rc.xml\n<a><command>x</command></a>\n"); err == nil {
		t.Fatalf("unsafe apply accepted")
	}
	got, _ = os.ReadFile(filepath.Join(s.Dir, "labwc", "output.xml"))
	if string(got) != "<outputs2/>\n" {
		t.Fatalf("refused apply changed the settings: %q", got)
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Fatalf("leftovers in %s: %v", s.Dir, entries)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "labwc")); !os.IsNotExist(err) {
		t.Fatalf("reset kept the settings")
	}
}

func TestBackground(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 32)...)
	if err := s.SetBackgroundColor("#12abEF"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBackgroundImage(png); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "background-color")); !os.IsNotExist(err) {
		t.Fatalf("the color survived an image")
	}
	info, err := os.Stat(filepath.Join(s.Dir, "background"))
	if err != nil || info.Mode().Perm() != 0o644 || info.Size() != int64(len(png)) {
		t.Fatalf("background = %v, %v", info, err)
	}
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	if !ImageKind(webp) || !ImageKind([]byte{0xff, 0xd8, 0xff, 0xe0}) || ImageKind([]byte("GIF89a")) {
		t.Fatalf("ImageKind is wrong")
	}
	for _, data := range [][]byte{nil, []byte("GIF89a....."), make([]byte, MaxImage+1)} {
		if err := s.SetBackgroundImage(data); !errors.Is(err, ErrInvalid) {
			t.Errorf("image of %d bytes accepted", len(data))
		}
	}
	for _, c := range []string{"red", "#12345", "#1234567", "#12345g"} {
		if err := s.SetBackgroundColor(c); !errors.Is(err, ErrInvalid) {
			t.Errorf("color %q accepted", c)
		}
	}
	if err := s.SetBackgroundColor("#000000"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "background")); !os.IsNotExist(err) {
		t.Fatalf("the image survived a color")
	}
	if err := s.ResetBackground(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 0 {
		t.Fatalf("reset left %v", entries)
	}
}

type deny struct{ asked []string }

func (d *deny) Check(sender dbus.Sender, action string) *dbus.Error {
	d.asked = append(d.asked, action)
	return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{"no"})
}

type allow struct{}

func (allow) Check(dbus.Sender, string) *dbus.Error { return nil }

func TestServiceAuthorization(t *testing.T) {
	dir := t.TempDir()
	d := &deny{}
	svc := NewService(Store{Dir: dir}, d)
	if e := svc.Apply(":1.5", payload); e == nil || e.Name != "org.freedesktop.DBus.Error.AccessDenied" {
		t.Fatalf("Apply = %v", e)
	}
	if e := svc.SetBackgroundColor(":1.5", "#000000"); e == nil {
		t.Fatalf("SetBackgroundColor allowed")
	}
	if len(d.asked) != 2 || d.asked[0] != Action {
		t.Fatalf("asked %v", d.asked)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("a refused call wrote %v", entries)
	}
	ok := NewService(Store{Dir: dir}, allow{})
	if e := ok.SetBackgroundColor(":1.5", "nope"); e == nil || e.Name != Iface+".Error.Invalid" {
		t.Fatalf("invalid color = %v", e)
	}
	if e := ok.Apply(":1.5", payload); e != nil {
		t.Fatal(e)
	}
}
