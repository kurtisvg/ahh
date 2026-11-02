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

After completing setup, run the same coverage gate used by CI:

```sh
go run ./tools/coverage
```

This runs the full suite with race detection and shuffled test order. It builds
one coverage-instrumented application for the browser tests, collects server and
wrapper subprocess counters, and merges them with Go test coverage. It needs no
model account. Coverage runs allow ten minutes for tests.

Each invocation writes a fresh `coverage/run.*` directory containing
`coverage.out`, `coverage.html`, and `summary.md`. Open the HTML file to inspect
uncovered code, or run `go tool cover -func=coverage/run.<id>/coverage.out`.
CI publishes the summary and uploads these reports even when the floor fails.
Test failures and missing coverage data fail the gate.

The gate measures application Go statements, including packages without tests.
It excludes `internal/store/testutils` and `tools/coverage`, which are test
infrastructure. JavaScript and optional installed-Codex tests are outside
this metric. Package percentages help locate gaps; only the weighted overall
percentage is enforced, using exact counts rather than rounded display values.

`tools/coverage/minimum.txt` contains the required whole percentage. Raise it
in reviewed changes as meaningful tests improve coverage, toward 95%; do not
lower it to accommodate uncovered features. Establish a higher floor with two
successful full runs and round the lower result down to a whole percent.
CI never updates this value automatically.

To check saved profiles without rerunning tests:

```sh
go run ./tools/coverage report -out coverage/rechecked.out coverage/run.<id>/tests.out coverage/run.<id>/subprocess.out
```

The report command uses the same minimum by default; `-minimum N` overrides it
for local comparisons. The full coverage run always uses the checked-in minimum.

`AHH_TEST_BINARY` and `AHH_COVERAGE_DIR` are test-only hooks set by the runner.
Normal `go test` runs continue to build their own browser-test binaries.

The initial 90% floor was established from two full runs measuring 90.92% and
90.84% combined coverage. Coverage reports retain the exact statement counts.
