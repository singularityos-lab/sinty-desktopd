# sinty-desktopd

Writes the few system files the Singularity desktop changes on behalf of the
person using it, the login screen settings and fingerprint reader drivers, and
reads the kernel log for the Logs app when the kernel restricts it. It
starts on demand through D-Bus activation and runs no other program.

## Who may call it

Root always. Otherwise polkit decides when it is on the bus, with the actions
`dev.sinty.login-screen`, `dev.sinty.fprint-driver` and
`dev.sinty.logs.read-kernel`. Without polkit, members
of `wheel`, `sudo` or `admin` are allowed. A refused caller gets
`org.freedesktop.DBus.Error.AccessDenied`.

## dev.sinty.LoginScreen1

Object `/dev/sinty/LoginScreen1`. Files live in `/var/lib/singularity/login-screen`.

| Member | Signature | |
| --- | --- | --- |
| `Apply` | `(s settings)` | `%%FILE rc.xml`, `%%FILE output.xml` and `%%FILE environment` sections, at most 256 KiB, swapped in atomically under `labwc/` |
| `Reset` | `()` | removes `labwc/` |
| `SetBackgroundImage` | `(ay image)` | PNG, JPEG or WebP, at most 25 MB |
| `SetBackgroundColor` | `(s color)` | `#RRGGBB` |
| `ResetBackground` | `()` | |

The XML files may not contain actions or commands, and `environment` may only
set `XKB_*`, `XCURSOR_THEME` and `XCURSOR_SIZE`. Bad input gets
`dev.sinty.LoginScreen1.Error.Invalid`.

## dev.sinty.FingerprintDriver1

Object `/dev/sinty/FingerprintDriver1`.

| Member | Signature | |
| --- | --- | --- |
| `Info` | `(s id) -> a{ss}` | `id`, `vendor`, `name`, `version`, `license`, `source`, `url`, `size`, `installed`; needs no authorization |
| `Install` | `(s id)` | |
| `Uninstall` | `(s id)` | |

`id` is the USB `vendor:product` of the sensor. Only drivers in the built-in
catalog can be installed. The package is downloaded over HTTPS and checked
against its size and SHA-256, then each file taken from it is checked again.
Libraries go to `/var/lib/singularity/fprint`, the device rule to
`/etc/udev/rules.d`. Afterwards the sensor is re-announced to udev and a
running `fprintd` is stopped so the next request starts it with the driver.

Errors: `dev.sinty.FingerprintDriver1.Error.Invalid`, `.NotFound`, `.Failed`.

## dev.sinty.Logs1

Object `/dev/sinty/Logs1`.

| Member | Signature | |
| --- | --- | --- |
| `ReadKernel` | `() -> s` | the kernel ring buffer from `/dev/kmsg`, one `<priority>[seconds.micro] message` line per record |

## Usage

    sinty-desktopd [-login-screen-dir DIR] [-fprint-dir DIR] [-udev-rules DIR]

## Build

    make build
    make static           # stripped static binary for the image
    make test

Requires Go >= 1.25. Dependencies: `github.com/godbus/dbus/v5`,
`github.com/klauspost/compress` and `github.com/ulikunitz/xz`.
