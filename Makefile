BIN := sinty-desktopd
PREFIX ?= /usr

.PHONY: all build static test vet install clean

all: build

build:
	go build -o $(BIN) ./cmd/sinty-desktopd

static:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/sinty-desktopd

test:
	go test ./...

vet:
	go vet ./...

install: static
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/libexec/$(BIN)
	install -Dm644 packaging/sinty-desktopd.conf $(DESTDIR)$(PREFIX)/share/dbus-1/system.d/sinty-desktopd.conf
	install -Dm644 packaging/dev.sinty.LoginScreen1.service $(DESTDIR)$(PREFIX)/share/dbus-1/system-services/dev.sinty.LoginScreen1.service
	install -Dm644 packaging/dev.sinty.FingerprintDriver1.service $(DESTDIR)$(PREFIX)/share/dbus-1/system-services/dev.sinty.FingerprintDriver1.service
	install -Dm644 packaging/dev.sinty.desktopd.policy $(DESTDIR)$(PREFIX)/share/polkit-1/actions/dev.sinty.desktopd.policy

clean:
	rm -f $(BIN)
