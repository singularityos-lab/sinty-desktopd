package fprint

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

const (
	MaxPackage    = 67108864
	ExtensionName = "singularity-fprint-drivers"
)

var (
	ErrInvalid  = errors.New("invalid")
	ErrNotFound = errors.New("not found")
	idPattern   = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{4}$`)
)

type Installer struct {
	Base      string
	UdevRules string
	Catalog   []Driver
	Arch      string
	Fetch     func(rawURL string, max int64) ([]byte, error)
	Restart   func(id string)
}

func New() *Installer {
	return &Installer{
		Base:      "/var/lib/singularity/fprint",
		UdevRules: "/etc/udev/rules.d",
		Catalog:   Catalog,
		Arch:      runtime.GOARCH,
		Fetch:     Download,
		Restart:   func(string) {},
	}
}

func (in *Installer) Lookup(id string) (*Driver, error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: invalid device id %q", ErrInvalid, id)
	}
	for i := range in.Catalog {
		if in.Catalog[i].ID == id {
			d := &in.Catalog[i]
			if d.Arch != "" && d.Arch != in.Arch {
				return nil, fmt.Errorf("%w: this driver only runs on %s", ErrInvalid, d.Arch)
			}
			return d, nil
		}
	}
	return nil, fmt.Errorf("%w: no driver is known for %s", ErrNotFound, id)
}

func (in *Installer) manifest(id string) string {
	return filepath.Join(in.Base, "installed", strings.ReplaceAll(id, ":", "-")+".list")
}

func (in *Installer) udevRule(id string) string {
	return filepath.Join(in.UdevRules, "60-singularity-fprint-"+strings.ReplaceAll(id, ":", "-")+".rules")
}

func (in *Installer) Installed(id string) bool {
	_, err := os.Stat(in.manifest(id))
	return err == nil
}

func (in *Installer) Info(id string) (map[string]string, error) {
	d, err := in.Lookup(id)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"id":        d.ID,
		"vendor":    d.Vendor,
		"name":      d.Name,
		"version":   d.Version,
		"license":   d.License,
		"source":    d.Source,
		"url":       d.URL,
		"size":      strconv.FormatInt(d.Size, 10),
		"installed": strconv.FormatBool(in.Installed(id)),
	}, nil
}

func Download(rawURL string, max int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("only https downloads are allowed")
	}
	client := &http.Client{
		Timeout:   300 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, Proxy: http.ProxyFromEnvironment},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) > 5 {
				return fmt.Errorf("refusing redirect to %s", req.URL)
			}
			return nil
		},
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("the download is larger than %d bytes", max)
	}
	return data, nil
}

func DataArchive(deb []byte) (string, []byte, error) {
	if !bytes.HasPrefix(deb, []byte("!<arch>\n")) {
		return "", nil, fmt.Errorf("the driver package is not an ar archive")
	}
	off := 8
	for off+60 <= len(deb) {
		header := deb[off : off+60]
		name := strings.TrimRight(string(header[0:16]), " /")
		size, err := strconv.Atoi(strings.TrimSpace(string(header[48:58])))
		if err != nil || size < 0 || off+60+size > len(deb) {
			return "", nil, fmt.Errorf("the driver package is damaged")
		}
		if strings.HasPrefix(name, "data.tar") {
			return name, deb[off+60 : off+60+size], nil
		}
		off += 60 + size + size%2
	}
	return "", nil, fmt.Errorf("the driver package has no data archive")
}

func Decompress(name string, data []byte) (io.Reader, func(), error) {
	switch name {
	case "data.tar":
		return bytes.NewReader(data), func() {}, nil
	case "data.tar.gz":
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
		return r, func() { r.Close() }, nil
	case "data.tar.xz":
		r, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
		return bufio.NewReader(r), func() {}, nil
	case "data.tar.zst":
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
		return r, r.Close, nil
	}
	return nil, nil, fmt.Errorf("unsupported driver package format")
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalize(member string) string {
	return "./" + strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(member, "./")), "/")
}

func Extract(deb []byte, files []File) (map[string][]byte, error) {
	name, data, err := DataArchive(deb)
	if err != nil {
		return nil, err
	}
	reader, done, err := Decompress(name, data)
	if err != nil {
		return nil, err
	}
	defer done()
	wanted := map[string]File{}
	for _, f := range files {
		wanted[normalize(f.Member)] = f
	}
	found := map[string][]byte{}
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the driver package is damaged: %v", err)
		}
		member := normalize(hdr.Name)
		f, ok := wanted[member]
		if !ok || hdr.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(tr, MaxPackage+1))
		if err != nil {
			return nil, err
		}
		if digest(content) != f.SHA256 {
			return nil, fmt.Errorf("%s failed the checksum", f.Member)
		}
		found[member] = content
	}
	for _, f := range files {
		if _, ok := found[normalize(f.Member)]; !ok {
			return nil, fmt.Errorf("the driver package is missing %s", f.Member)
		}
	}
	return found, nil
}

func place(dest string, content []byte, list *[]string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest+".new", content, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(dest+".new", 0o644); err != nil {
		return err
	}
	if err := os.Rename(dest+".new", dest); err != nil {
		return err
	}
	*list = append(*list, dest)
	return nil
}

func (in *Installer) Install(id string) error {
	d, err := in.Lookup(id)
	if err != nil {
		return err
	}
	deb, err := in.Fetch(d.URL, MaxPackage)
	if err != nil {
		return fmt.Errorf("the driver could not be downloaded: %v", err)
	}
	if int64(len(deb)) != d.Size {
		return fmt.Errorf("the downloaded driver has an unexpected size")
	}
	if digest(deb) != d.SHA256 {
		return fmt.Errorf("the downloaded driver failed the checksum")
	}
	contents, err := Extract(deb, d.Files)
	if err != nil {
		return err
	}
	var list []string
	for _, f := range d.Files {
		content := contents[normalize(f.Member)]
		var dest string
		switch f.Kind {
		case "tod":
			dest = filepath.Join(in.Base, "tod-1", path.Base(f.Member))
		case "usr":
			dest = filepath.Join(in.Base, ExtensionName, strings.TrimPrefix(normalize(f.Member), "./"))
		case "udev":
			dest = in.udevRule(id)
		default:
			return fmt.Errorf("unknown file kind %s", f.Kind)
		}
		if err := place(dest, content, &list); err != nil {
			return err
		}
	}
	release := filepath.Join(in.Base, ExtensionName, "usr/lib/extension-release.d", "extension-release."+ExtensionName)
	if err := os.MkdirAll(filepath.Dir(release), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(release, []byte("ID=_any\n"), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(in.manifest(id)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(in.manifest(id), []byte(strings.Join(list, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	in.Restart(id)
	return nil
}

func (in *Installer) owns(p string) bool {
	clean := filepath.Clean(p)
	if strings.HasPrefix(clean, filepath.Clean(in.Base)+"/") {
		return true
	}
	return strings.HasPrefix(clean, filepath.Join(in.UdevRules, "60-singularity-fprint-"))
}

func (in *Installer) Uninstall(id string) error {
	if _, err := in.Lookup(id); err != nil {
		return err
	}
	data, err := os.ReadFile(in.manifest(id))
	if err != nil {
		return fmt.Errorf("%w: the driver for %s is not installed", ErrNotFound, id)
	}
	for _, p := range strings.Split(string(data), "\n") {
		if p == "" || !in.owns(p) {
			continue
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(in.manifest(id)); err != nil {
		return err
	}
	in.Restart(id)
	return nil
}
