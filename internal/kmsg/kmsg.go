package kmsg

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type Record struct {
	Priority int
	Usec     uint64
	Message  string
}

func Parse(raw string) (Record, bool) {
	head, body, ok := strings.Cut(raw, ";")
	if !ok {
		return Record{}, false
	}
	fields := strings.Split(head, ",")
	if len(fields) < 3 {
		return Record{}, false
	}
	pri, err1 := strconv.Atoi(fields[0])
	usec, err2 := strconv.ParseUint(fields[2], 10, 64)
	if err1 != nil || err2 != nil {
		return Record{}, false
	}
	message, _, _ := strings.Cut(body, "\n")
	return Record{Priority: pri, Usec: usec, Message: Unescape(message)}, true
}

func Unescape(s string) string {
	if !strings.Contains(s, `\x`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' {
			if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				if v == '\n' || v == '\t' || v >= 0x20 {
					b.WriteByte(byte(v))
				} else {
					b.WriteByte(' ')
				}
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func Format(records []Record) string {
	var b strings.Builder
	for _, r := range records {
		for _, line := range strings.Split(r.Message, "\n") {
			fmt.Fprintf(&b, "<%d>[%5d.%06d] %s\n", r.Priority, r.Usec/1000000, r.Usec%1000000, line)
		}
	}
	return b.String()
}

func Read(path string) ([]Record, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer syscall.Close(fd)
	var records []Record
	buf := make([]byte, 16384)
	for {
		n, err := syscall.Read(fd, buf)
		if errors.Is(err, syscall.EPIPE) {
			continue
		}
		if errors.Is(err, syscall.EAGAIN) {
			return records, nil
		}
		if err != nil {
			return records, &os.PathError{Op: "read", Path: path, Err: err}
		}
		if n <= 0 {
			return records, nil
		}
		if r, ok := Parse(string(buf[:n])); ok {
			records = append(records, r)
		}
	}
}
