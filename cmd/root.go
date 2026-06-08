package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/version"

	flag "github.com/spf13/pflag"
)

type options struct {
	host    string
	port    string
	version bool
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("ahh", flag.ContinueOnError)
	fs.StringVar(&opts.host, "host", "127.0.0.1", "HTTP listen host")
	fs.StringVar(&opts.port, "port", "8080", "HTTP listen port")
	fs.BoolVar(&opts.version, "version", false, "Print version and exit")
	err := fs.Parse(args)
	return opts, err
}

func Execute() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(1)
	}

	if opts.version {
		fmt.Println(version.Version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Ahh %s -- the agent harness harness\n", version.Version)
	ln, err := server.Listen(opts.host, opts.port)
	if err != nil {
		slog.Error("listen error", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Listening on http://%s\n", ln.Addr().String())

	if err := server.Serve(ctx, ln); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
