# Development

Ahh runs a local harness terminal in your browser.
Run the commands below from the repository root.

## Setup

- [Go](https://go.dev/doc/install) >= 1.27.3 and a C compiler for race tests.
- [Node.js](https://nodejs.org/en/download) from `.nvmrc`, with its bundled npm.
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/) v2.14.0.

Install dependencies after checkout or when their versions change:

```sh
npm --prefix internal/server/web ci
go generate ./...
go run github.com/mxschmitt/playwright-go/cmd/playwright install --with-deps chromium
```

`go generate` builds the browser assets. Run it again after editing browser
sources; `go build` and `go run` do not run it automatically. The binary embeds
the generated files, so Node and npm are only needed during development and builds.

## Run

```sh
go generate ./...
go run . --help
go run . --version
go run . serve --address 127.0.0.1 --port 8080 --workspace /tmp/workspace
go run . wrap codex --address 127.0.0.1 --port 8081
```

### Workspace

The workspace is Ahh's storage root for Project repositories and metadata.
`serve` creates it if needed, with this initial structure:

```text
<workspace>/
  .ahh.lock   # Server ownership lock
  ahh.db      # Project metadata
  projects/   # Project repository storage
```

Select it with `--workspace PATH` (default: `./ahh-workspace`). Relative paths
resolve from the startup directory; the resolved path is printed on stderr.

## Build

Build the binary in the repository root:

```sh
go generate ./...
go build .
./ahh --version
```

The binary embeds [internal/version/version.txt](internal/version/version.txt)
and runs outside the checkout. `--version` prints `ahh version 0.0.0`.

## Code layout

The project uses one Go module. Tests live beside the code they exercise.

| Location | Responsibility |
| --- | --- |
| `main.go` | Process entry point; calls `cmd.Execute()`. |
| `cmd/` | Cobra commands, arguments, I/O, and exit handling. |
| `internal/version/` | Version text embedded in the binary. |
| `internal/server/` | Orchestrator HTTP server and its lifetime. |
| `internal/server/web/` | Browser sources, asset build, and browser tests. |
| `internal/wrapper/` | HTTP/WebSocket server, terminal attachments, and harness lifetime. |
| `internal/harness/` | Harness selection and lifecycle interface. |
| `internal/harness/codex/` | Codex startup and configuration. |
| `internal/harness/pty/` | Shared process and PTY lifecycle. |

Keep commands thin. Add application behavior under `internal/` when it has an
owner and a concrete need. Add subcommands through their own constructors when
needed. Tests configure arguments and I/O directly on fresh Cobra commands.

## Lint

Format code and apply available lint fixes:

```sh
golangci-lint fmt
golangci-lint run --fix --timeout 5m
```

## Test

Run all tests, including browser tests, with race detection:

```sh
go test -count=1 -race -shuffle=on -timeout 5m ./...
```

Browser tests use a fake harness and run the built binary outside the checkout.
They check direct connections and a path-prefix proxy, without a model account.
Use `-short` to skip browser tests, or `-run TestTerminalReconnect` to select a
scenario. Keep `-count=1` because the browser tests build the application as a
subprocess. Add `-artifacts -outputdir <existing-directory>` to retain screenshots
and failure traces.

### Test with installed harnesses

Harness integration tests are opt-in. With Codex installed and logged in,
include its smoke test with:

```sh
go generate ./...
go test -tags=integration_codex -count=1 -race -shuffle=on -timeout 5m ./...
```

The Codex test checks startup, terminal rendering, resizing, and shutdown in a
temporary Git directory. It sends no model prompt.

### Coverage

Run the CI coverage gate locally:

```sh
go run ./tools/coverage
```

This runs the full suite and measures application Go statements, including code
exercised by browser subprocesses. Open `coverage/run.*/coverage.html` to inspect
uncovered code; CI publishes the same reports as artifacts.

The minimum is set in [tools/coverage/minimum.txt](tools/coverage/minimum.txt).
Raise it toward 95% as tests improve: take the lower of two successful full runs
and round down to a whole percent. Do not lower it to accommodate uncovered code.

See `go run ./tools/coverage report -h` for options to recheck saved profiles.
