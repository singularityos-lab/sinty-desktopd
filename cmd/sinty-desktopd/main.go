package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/godbus/dbus/v5"

	"github.com/singularityos-lab/sinty-desktopd/internal/auth"
	"github.com/singularityos-lab/sinty-desktopd/internal/fprint"
	"github.com/singularityos-lab/sinty-desktopd/internal/kmsg"
	"github.com/singularityos-lab/sinty-desktopd/internal/loginscreen"
)

func main() {
	loginDir := flag.String("login-screen-dir", "/var/lib/singularity/login-screen", "where the login screen settings are kept")
	fprintDir := flag.String("fprint-dir", "/var/lib/singularity/fprint", "where fingerprint drivers are installed")
	udevDir := flag.String("udev-rules", "/etc/udev/rules.d", "where device rules are written")
	flag.Parse()
	log.SetFlags(0)

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		log.Fatalf("system bus: %v", err)
	}
	defer conn.Close()
	authorizer := &auth.Authorizer{Conn: conn}

	login := loginscreen.NewService(loginscreen.Store{Dir: *loginDir}, authorizer)
	if err := login.Export(conn); err != nil {
		log.Fatalf("export %s: %v", loginscreen.Name, err)
	}
	installer := fprint.New()
	installer.Base = *fprintDir
	installer.UdevRules = *udevDir
	installer.Restart = fprint.Restarter(conn)
	if err := fprint.NewService(installer, authorizer).Export(conn); err != nil {
		log.Fatalf("export %s: %v", fprint.Name, err)
	}
	if err := kmsg.NewService("/dev/kmsg", authorizer).Export(conn); err != nil {
		log.Fatalf("export %s: %v", kmsg.Name, err)
	}
	for _, name := range []string{loginscreen.Name, fprint.Name, kmsg.Name} {
		reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
		if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
			log.Fatalf("cannot own %s: %v", name, err)
		}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
