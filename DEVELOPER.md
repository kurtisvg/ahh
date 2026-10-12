# Development

Ahh currently provides CLI help and version reporting. Run the commands below
from the repository root.

## Setup

- [Go](https://go.dev/doc/install) >= 1.27.2.
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/) v2.14.0.

## Run

```sh
go run . --help
go run . --version
```

## Build

Build the binary in the repository root:

```sh
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

Keep commands thin. Add application behavior under `internal/` when it has an
owner and a concrete need. Add subcommands through their own constructors when
needed. Tests configure arguments and I/O directly on fresh Cobra commands.

## Lint

Format code and apply available lint fixes:

```sh
golangci-lint run --fix --timeout 5m
```

## Test

Run tests with race detection:

```sh
go test -race -shuffle=on -timeout 60s ./...
```
