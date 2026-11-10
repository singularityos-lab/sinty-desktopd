package fprint

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func tarball(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	tw.WriteHeader(&tar.Header{Name: "./usr/", Typeflag: tar.TypeDir, Mode: 0o755})
	for _, n := range names {
		tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(files[n]))})
		tw.Write([]byte(files[n]))
	}
	tw.WriteHeader(&tar.Header{Name: "./usr/lib/link.so", Typeflag: tar.TypeSymlink, Linkname: "/etc/shadow"})
	tw.Close()
	return buf.Bytes()
}

func compress(t *testing.T, kind string, data []byte) []byte {
	var buf bytes.Buffer
	switch kind {
	case "data.tar":
		return data
	case "data.tar.gz":
		w := gzip.NewWriter(&buf)
		w.Write(data)
		w.Close()
	case "data.tar.xz":
		w, err := xz.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		w.Close()
	case "data.tar.zst":
		w, err := zstd.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		w.Close()
	}
	return buf.Bytes()
}

func member(name string, data []byte) []byte {
	header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", name, "0", "0", "0", "100644", len(data))
	out := append([]byte(header), data...)
	if len(data)%2 == 1 {
		out = append(out, '\n')
	}
	return out
}

func deb(t *testing.T, kind string, files map[string]string) []byte {
	out := []byte("!<arch>\n")
	out = append(out, member("debian-binary", []byte("2.0\n"))...)
	out = append(out, member("control.tar.gz", []byte("x"))...)
	out = append(out, member(kind, compress(t, kind, tarball(t, files)))...)
	return out
}

var payload = map[string]string{
	"./usr/lib/x86_64-linux-gnu/libfprint-2/tod-1/libtod-test.so": "tod library",
	"./usr/lib/x86_64-linux-gnu/libhelper-test.so":                 "helper library",
	"./lib/udev/rules.d/60-test.rules":                             "SUBSYSTEM==\"usb\"\n",
	"./usr/share/doc/unrelated":                                    "ignored",
}

func fixture(t *testing.T, kind string) (*Installer, *[]string) {
	pkg := deb(t, kind, payload)
	restarted := []string{}
	in := &Installer{
		Base:      filepath.Join(t.TempDir(), "fprint"),
		UdevRules: filepath.Join(t.TempDir(), "rules.d"),
		Arch:      "amd64",
		Catalog: []Driver{{
			ID: "abcd:1234", Name: "Test driver", Version: "1", URL: "https://drivers.example/test.deb",
			SHA256: sum(pkg), Size: int64(len(pkg)), Arch: "amd64",
			Files: []File{
				{"tod", "./usr/lib/x86_64-linux-gnu/libfprint-2/tod-1/libtod-test.so", sum([]byte("tod library"))},
				{"usr", "usr/lib/x86_64-linux-gnu/libhelper-test.so", sum([]byte("helper library"))},
				{"udev", "./lib/udev/rules.d/60-test.rules", sum([]byte("SUBSYSTEM==\"usb\"\n"))},
			},
		}, {ID: "abcd:9999", Arch: "arm64"}},
		Fetch: func(u string, max int64) ([]byte, error) {
			if u != "https://drivers.example/test.deb" || max != MaxPackage {
				return nil, errors.New("unexpected fetch")
			}
			return pkg, nil
		},
	}
	in.Restart = func(id string) { restarted = append(restarted, id) }
	return in, &restarted
}

func read(t *testing.T, p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return string(data)
}

func TestInstallUninstall(t *testing.T) {
	for _, kind := range []string{"data.tar", "data.tar.gz", "data.tar.xz", "data.tar.zst"} {
		t.Run(kind, func(t *testing.T) {
			in, restarted := fixture(t, kind)
			info, err := in.Info("abcd:1234")
			if err != nil || info["installed"] != "false" || info["name"] != "Test driver" {
				t.Fatalf("Info = %v, %v", info, err)
			}
			if err := in.Install("abcd:1234"); err != nil {
				t.Fatal(err)
			}
			tod := filepath.Join(in.Base, "tod-1", "libtod-test.so")
			usr := filepath.Join(in.Base, ExtensionName, "usr/lib/x86_64-linux-gnu/libhelper-test.so")
			rule := filepath.Join(in.UdevRules, "60-singularity-fprint-abcd-1234.rules")
			if read(t, tod) != "tod library" || read(t, usr) != "helper library" || !strings.HasPrefix(read(t, rule), "SUBSYSTEM") {
				t.Fatalf("files not placed")
			}
			if read(t, filepath.Join(in.Base, ExtensionName, "usr/lib/extension-release.d", "extension-release."+ExtensionName)) != "ID=_any\n" {
				t.Fatalf("no extension release")
			}
			if _, err := os.Stat(filepath.Join(in.Base, "unrelated")); !os.IsNotExist(err) {
				t.Fatalf("unlisted member extracted")
			}
			if info, _ := in.Info("abcd:1234"); info["installed"] != "true" {
				t.Fatalf("not reported installed")
			}
			outside := filepath.Join(t.TempDir(), "keep")
			os.WriteFile(outside, []byte("x"), 0o644)
			f, _ := os.OpenFile(in.manifest("abcd:1234"), os.O_APPEND|os.O_WRONLY, 0)
			f.WriteString(outside + "\n")
			f.Close()
			if err := in.Uninstall("abcd:1234"); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{tod, usr, rule, in.manifest("abcd:1234")} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatalf("%s survived uninstall", p)
				}
			}
			if _, err := os.Stat(outside); err != nil {
				t.Fatalf("uninstall removed a file outside its directories")
			}
			if len(*restarted) != 2 {
				t.Fatalf("restarted %v", *restarted)
			}
			if err := in.Uninstall("abcd:1234"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("second uninstall = %v", err)
			}
		})
	}
}

func TestRefusals(t *testing.T) {
	in, restarted := fixture(t, "data.tar.gz")
	for id, want := range map[string]error{"ABCD:1234": ErrInvalid, "abcd:1234;x": ErrInvalid, "0000:0000": ErrNotFound, "abcd:9999": ErrInvalid} {
		if err := in.Install(id); !errors.Is(err, want) {
			t.Errorf("Install(%s) = %v", id, err)
		}
	}
	good := in.Catalog[0]
	in.Catalog[0].SHA256 = strings.Repeat("0", 64)
	if err := in.Install("abcd:1234"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("bad package checksum = %v", err)
	}
	in.Catalog[0] = good
	in.Catalog[0].Size++
	if err := in.Install("abcd:1234"); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("bad size = %v", err)
	}
	in.Catalog[0] = good
	in.Catalog[0].Files = append([]File{}, good.Files...)
	in.Catalog[0].Files[1].SHA256 = strings.Repeat("1", 64)
	if err := in.Install("abcd:1234"); err == nil || !strings.Contains(err.Error(), "failed the checksum") {
		t.Fatalf("bad member checksum = %v", err)
	}
	in.Catalog[0] = good
	in.Catalog[0].Files = append([]File{}, good.Files...)
	in.Catalog[0].Files[0].Member = "./usr/lib/missing.so"
	if err := in.Install("abcd:1234"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing member = %v", err)
	}
	if _, err := os.Stat(in.UdevRules); !os.IsNotExist(err) {
		t.Fatalf("a refused install wrote rules")
	}
	if len(*restarted) != 0 {
		t.Fatalf("restarted after refusals")
	}
	if _, _, err := DataArchive([]byte("PK\x03\x04")); err == nil {
		t.Fatalf("zip accepted as deb")
	}
	if _, _, err := DataArchive(append([]byte("!<arch>\n"), []byte("data.tar        0           0     0     100644  99999     `\n")...)); err == nil {
		t.Fatalf("truncated member accepted")
	}
}

func TestSymlinkMember(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "./lib/udev/rules.d/60-test.rules", Typeflag: tar.TypeSymlink, Linkname: "/etc/shadow"})
	tw.Close()
	pkg := append([]byte("!<arch>\n"), member("data.tar", buf.Bytes())...)
	_, err := Extract(pkg, []File{{"udev", "./lib/udev/rules.d/60-test.rules", sum(nil)}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("symlink member = %v", err)
	}
}

func TestTrigger(t *testing.T) {
	sys := t.TempDir()
	mk := func(name, vendor, product string) string {
		dir := filepath.Join(sys, "bus", "usb", "devices", name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "idVendor"), []byte(vendor+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "idProduct"), []byte(product+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "uevent"), nil, 0o644)
		return dir
	}
	hit := mk("1-2", "10a5", "9800")
	miss := mk("1-3", "10a5", "9801")
	if n := Trigger(sys, "10a5:9800"); n != 1 {
		t.Fatalf("Trigger = %d", n)
	}
	if read(t, filepath.Join(hit, "uevent")) != "add" || read(t, filepath.Join(miss, "uevent")) != "" {
		t.Fatalf("wrong device triggered")
	}
}

type deny struct{ count int }

func (d *deny) Check(dbus.Sender, string) *dbus.Error {
	d.count++
	return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{"no"})
}

func TestServiceAuthorization(t *testing.T) {
	in, restarted := fixture(t, "data.tar.zst")
	d := &deny{}
	svc := NewService(in, d)
	if e := svc.Install(":1.9", "abcd:1234"); e == nil || e.Name != "org.freedesktop.DBus.Error.AccessDenied" {
		t.Fatalf("Install = %v", e)
	}
	if e := svc.Install(":1.9", "nope"); e == nil || e.Name != Iface+".Error.Invalid" {
		t.Fatalf("invalid id = %v", e)
	}
	if d.count != 1 {
		t.Fatalf("auth asked %d times", d.count)
	}
	if _, err := os.Stat(in.Base); !os.IsNotExist(err) || len(*restarted) != 0 {
		t.Fatalf("a refused install touched the system")
	}
	info, e := svc.Info("abcd:1234")
	if e != nil || info["id"] != "abcd:1234" {
		t.Fatalf("Info = %v, %v", info, e)
	}
}
