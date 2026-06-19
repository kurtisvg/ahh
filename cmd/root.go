package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/version"

	flag "github.com/spf13/pflag"
)

type command struct {
	name          string
	serverOptions serverOptions
}

type serverOptions struct {
	host string
	port string
}

func parseCommand(args []string) (command, error) {
	var version bool
	root := flag.NewFlagSet("ahh", flag.ContinueOnError)
	root.SetInterspersed(false)
	root.BoolVar(&version, "version", false, "Print version and exit")
	if err := root.Parse(args); err != nil {
		return command{}, err
	}
	if version {
		return command{name: "version"}, nil
	}

	remaining := root.Args()
	if len(remaining) == 0 {
		return command{}, errors.New("missing command")
	}

	switch remaining[0] {
	case "server":
		opts, err := parseServerOptions(remaining[1:])
		if err != nil {
			return command{}, err
		}
		return command{name: "server", serverOptions: opts}, nil
	case "run-wrapper":
		return command{name: "run-wrapper"}, nil
	default:
		return command{}, fmt.Errorf("unknown command %q", remaining[0])
	}
}

func parseServerOptions(args []string) (serverOptions, error) {
	var opts serverOptions
	fs := flag.NewFlagSet("ahh", flag.ContinueOnError)
	fs.StringVar(&opts.host, "host", "127.0.0.1", "HTTP listen host")
	fs.StringVar(&opts.port, "port", "8080", "HTTP listen port")
	err := fs.Parse(args)
	return opts, err
}

// Execute parses CLI flags and starts the Ahh server.
func Execute() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd, err := parseCommand(args)
	if err != nil {
		printUsage(stderr)
		return err
	}

	switch cmd.name {
	case "version":
		fmt.Fprintln(stdout, version.Version)
		return nil
	case "server":
		return runServer(ctx, stdout, cmd.serverOptions)
	case "run-wrapper":
		return errors.New("run-wrapper is not implemented yet")
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", cmd.name)
	}
}

func runServer(ctx context.Context, stdout io.Writer, opts serverOptions) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(stdout, "Ahh %s -- the agent harness harness\n", version.Version)
	ln, err := server.Listen(opts.host, opts.port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Fprintf(stdout, "Listening on http://%s\n", ln.Addr().String())

	if err := server.Serve(ctx, ln); err != nil {
		slog.Error("server error", "error", err)
		return err
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  ahh --version
  ahh server [--host 127.0.0.1] [--port 8080]
  ahh run-wrapper claude-code`)
}
