# Contributing to sinty-desktopd

Thanks for your interest in contributing!

## Quick Start

```bash
git clone https://github.com/singularityos-lab/sinty-desktopd
cd sinty-desktopd
go build ./...
go test ./...
```

## Guidelines

- Keep changes additive and tested. sinty-desktopd runs as root and writes
  system files on behalf of the desktop; prefer small, verifiable commits over large
  rewrites.
- Match the surrounding style. Comments explain WHY, not WHAT.
- By submitting a contribution you agree to the [CLA](CLA.md).

## License

GPL-3.0-only.
