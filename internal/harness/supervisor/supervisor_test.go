package supervisor

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	harnessserver "github.com/kurtisvg/ahh/internal/harness/server"
)

func TestSupervisorStartsAndStopsWrapper(t *testing.T) {
	t.Parallel()

	supervisor := New(Options{
		CommandPath: os.Args[0],
		ExtraArgs:   []string{"-test.run=TestHelperProcess", "--"},
		Env:         []string{"GO_WANT_HELPER_PROCESS=1"},
		Harness:     "claude-code",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}

	status := supervisor.Status()
	if status.Harness != "claude-code" {
		t.Fatalf("harness = %q, want %q", status.Harness, "claude-code")
	}
	if status.State != StateReady {
		t.Fatalf("state = %q, want %q", status.State, StateReady)
	}
	if status.Address == "" {
		t.Fatal("address is empty")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := supervisor.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	if got := supervisor.Status().State; got != StateStopped {
		t.Fatalf("state = %q, want %q", got, StateStopped)
	}
}

func TestReservePort(t *testing.T) {
	t.Parallel()

	port, err := reservePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if port == "" {
		t.Fatal("port is empty")
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := helperArgs(os.Args)
	if len(args) != 6 || args[0] != "run" || args[1] != "claude-code" {
		os.Exit(2)
	}

	host, port := "", ""
	for i := 2; i < len(args); i += 2 {
		switch args[i] {
		case "--host":
			host = args[i+1]
		case "--port":
			port = args[i+1]
		default:
			os.Exit(2)
		}
	}
	if host == "" || port == "" {
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		os.Exit(2)
	}
	if err := harnessserver.Serve(ctx, ln, "claude-code"); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func helperArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	for i, arg := range args {
		if arg == "run" {
			return args[i:]
		}
	}
	return nil
}
