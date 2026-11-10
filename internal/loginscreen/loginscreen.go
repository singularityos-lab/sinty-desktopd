package loginscreen

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	MaxPayload = 262144
	MaxImage   = 26214400
)

var ErrInvalid = errors.New("invalid")

type Store struct {
	Dir string
}

func invalid(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func (s Store) target() string {
	return filepath.Join(s.Dir, "labwc")
}

func (s Store) Reset() error {
	return os.RemoveAll(s.target())
}

var (
	colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	unsafeXML    = regexp.MustCompile(`(?i)<action|execute|<command`)
	envPattern   = regexp.MustCompile(`^(XKB_[A-Z_]+|XCURSOR_(THEME|SIZE))=[A-Za-z0-9_.,:+-]*$`)
)

func Parse(payload string) (map[string]string, error) {
	if len(payload) > MaxPayload {
		return nil, invalid("the settings are larger than %d bytes", MaxPayload)
	}
	files := map[string]string{}
	current := ""
	var body strings.Builder
	flush := func() {
		if current != "" {
			files[current] = body.String()
		}
		body.Reset()
	}
	for _, line := range strings.SplitAfter(payload, "\n") {
		if line == "" {
			continue
		}
		text := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(text, "%%FILE ") {
			flush()
			current = strings.TrimPrefix(text, "%%FILE ")
			switch current {
			case "rc.xml", "output.xml", "environment":
			default:
				return nil, invalid("unexpected file %s", current)
			}
			continue
		}
		if current == "" {
			return nil, invalid("data before the first file")
		}
		body.WriteString(text + "\n")
	}
	flush()
	for _, name := range []string{"rc.xml", "output.xml"} {
		content, ok := files[name]
		if !ok {
			continue
		}
		if unsafeXML.MatchString(content) {
			return nil, invalid("%s may not contain actions", name)
		}
		if !strings.HasPrefix(content, "<") {
			return nil, invalid("%s is not XML", name)
		}
	}
	if env, ok := files["environment"]; ok {
		for _, line := range strings.Split(strings.TrimSuffix(env, "\n"), "\n") {
			if !envPattern.MatchString(line) {
				return nil, invalid("environment may only set keyboard and cursor variables")
			}
		}
	}
	return files, nil
}

func (s Store) Apply(payload string) error {
	files, err := Parse(payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(s.Dir, ".login-screen.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(work, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if err := os.Chmod(work, 0o755); err != nil {
		return err
	}
	target := s.target()
	old := target + ".old"
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if err := os.Rename(work, target); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

func ImageKind(data []byte) bool {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return true
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return true
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return true
	}
	return false
}

func (s Store) SetBackgroundImage(data []byte) error {
	if len(data) == 0 {
		return invalid("no image received")
	}
	if len(data) > MaxImage {
		return invalid("the image is larger than 25 MB")
	}
	if !ImageKind(data) {
		return invalid("only PNG, JPEG and WebP images are supported")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".background.")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.Dir, "background")); err != nil {
		return err
	}
	return removeIfPresent(filepath.Join(s.Dir, "background-color"))
}

func (s Store) SetBackgroundColor(color string) error {
	color = strings.TrimSpace(color)
	if !colorPattern.MatchString(color) {
		return invalid("invalid color")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "background-color"), []byte(color+"\n"), 0o644); err != nil {
		return err
	}
	return removeIfPresent(filepath.Join(s.Dir, "background"))
}

func (s Store) ResetBackground() error {
	if err := removeIfPresent(filepath.Join(s.Dir, "background")); err != nil {
		return err
	}
	return removeIfPresent(filepath.Join(s.Dir, "background-color"))
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
