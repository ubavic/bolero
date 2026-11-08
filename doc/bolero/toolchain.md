# Toolchain identity

The Bolero toolchain is a fork of the Go toolchain, but it presents itself as
its own product. It is configured only through its own environment variables,
it installs its own executables, and its messages do not mention Go.

This document lists what is renamed, and what deliberately keeps its Go name.

## Commands

| Go | Bolero | Source |
|---|---|---|
| `go` | `bolero` | `cmd/go` |
| `gofmt` | `bolerofmt` | `cmd/gofmt` |

The source directories keep their upstream names. Only the installed
executables, the usage lines, the help texts and the messages change.
`go build cmd/go` produces an executable named `bolero`;
`internal/bolero.ExecName` is the single place that maps the import path to the
executable name.

Other tools (`compile`, `link`, `vet`, `asm`, `cgo`, ...) keep their names and
are run through `bolero tool <name>`.

## Environment variables

The toolchain reads the following variables and ignores their `GO*` upstream
counterparts. There is no fallback: a program that sets `GOFLAGS` configures
nothing.

| Go | Bolero |
|---|---|
| `GOROOT` | `BOLERO_ROOT` |
| `GOPATH` | `BOLERO_PATH` |
| `GOBIN` | `BOLERO_BIN` |
| `GOCACHE` | `BOLERO_CACHE` |
| `GOCACHEPROG` | `BOLERO_CACHEPROG` |
| `GOENV` | `BOLERO_ENV` |
| `GOFLAGS` | `BOLERO_FLAGS` |
| `GOMOD` | `BOLERO_MOD` |
| `GOMODCACHE` | `BOLERO_MODCACHE` |
| `GO111MODULE` | `BOLERO_MODULE` |
| `GOWORK` | `BOLERO_WORK` |
| `GOPROXY` | `BOLERO_PROXY` |
| `GONOPROXY` | `BOLERO_NOPROXY` |
| `GOSUMDB` | `BOLERO_SUMDB` |
| `GONOSUMDB` | `BOLERO_NOSUMDB` |
| `GOPRIVATE` | `BOLERO_PRIVATE` |
| `GOINSECURE` | `BOLERO_INSECURE` |
| `GOVCS` | `BOLERO_VCS` |
| `GOAUTH` | `BOLERO_AUTH` |
| `GOTMPDIR` | `BOLERO_TMPDIR` |
| `GOTOOLCHAIN` | `BOLERO_TOOLCHAIN` |
| `GOTOOLDIR` | `BOLERO_TOOLDIR` |
| `GOTELEMETRY` | `BOLERO_TELEMETRY` |
| `GOTELEMETRYDIR` | `BOLERO_TELEMETRYDIR` |
| `GOVERSION` | `BOLERO_VERSION` |

`bolero env` reports the new names, `bolero env -w` writes them, and the
`-json`, `-changed` and `-u` flags work as in Go.

The variables that describe the build target or configure the compiler, and the
variables of the run-time system, are not part of the toolchain configuration
and keep their names: `GOOS`, `GOARCH`, `GOAMD64` and the other architecture
variables, `GOEXPERIMENT`, `GOFIPS140`, `CGO_ENABLED`, `CGO_CFLAGS` and the
other `CGO_*` variables, `GODEBUG`, `GOGC`, `GOMAXPROCS` and the like.

## Files and directories

| Go | Bolero |
|---|---|
| `$GOROOT/go.env` | `$BOLERO_ROOT/bolero.env` |
| `<user config dir>/go/env` | `<user config dir>/bolero/env` |
| `<user config dir>/go/telemetry` | `<user config dir>/bolero/telemetry` |
| `<user cache dir>/go-build` | `<user cache dir>/bolero-build` |
| `$HOME/go` (default `GOPATH`) | `$HOME/bolero` (default `BOLERO_PATH`) |

`bolero.env` holds the defaults of the toolchain configuration. Its
`BOLERO_TOOLCHAIN=local` line keeps a build from ever being handed to an
upstream toolchain.

## Messages

Usage lines, help topics and messages of `bolero`, `bolerofmt`, `compile`,
`link`, `asm`, `cgo`, `vet`, `cover`, `objdump`, `pack` and `covdata` say
"Bolero" and `bolero`, and error messages that suggest a command suggest the
`bolero` command (`bolero mod tidy`, `bolero get ...`). Error messages are
prefixed with `bolero:`. `bolero bug` reports to the Bolero issue tracker.

## What keeps its Go name

These are file formats, language-level names and upstream data, not
properties of the toolchain:

- `go.mod`, `go.sum`, `go.work` and the `go` and `toolchain` lines of `go.mod`.
- `//go:build`, `//go:embed`, `//go:generate` and the other `//go:` directives.
- `.go` source files and the `GoFiles` family of `go list` fields.
- Version strings (`go1.26.3`), the `go version` line recorded in executables,
  and messages about the Go language version of a module.
- The `$GOROOT` placeholder inside object files and export data.
- The header of fuzz corpus files (`go test fuzz v1`).
- The upstream standard library, including its `//go:generate go run ...` lines.

## Building the toolchain

`make.bash` and `cmd/dist` still need an upstream Go toolchain as bootstrap.
It is configured with `GOROOT_BOOTSTRAP`, and `cmd/dist` hands upstream `GO*`
variables to it and `BOLERO_*` variables to everything it builds. The remaining
build knobs of `make.bash` keep their names (`GO_GCFLAGS`, `GO_LDFLAGS`,
`GO_DISTFLAGS`, `GO_EXTLINK_ENABLED`, `GOOS`, `GOARCH`, `CGO_ENABLED`).

## Known deviations from a clean separation

- `runtime.Version()` is still the upstream version, so `bolero version`
  prints `bolero version go1.26.3 linux/amd64`.
- `golang.org/x/telemetry` is vendored with a hand patch that changes its
  directory name to `bolero`. It must be reapplied when it is re-vendored; the
  non-short `cmd/internal/moddeps` test reports it.
- The upstream standard library keeps its `//go:generate go ...` lines, so
  regenerating it needs the `go` command of an upstream toolchain on `PATH`.
