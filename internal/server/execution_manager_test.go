package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecutionManagerLifecycle(t *testing.T) {
	useTestWrapper(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var launches atomic.Int32
	manager := newExecutionManager(func(ctx context.Context, directory string) (*wrapperProcess, error) {
		if launches.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return startWrapper(ctx, directory, os.Args[0], "-test.run=^TestChildWrapper$")
	})
	t.Cleanup(func() { _ = manager.Close(ctx) })
	directory := t.TempDir()
	if err := manager.Start(ctx, "a", directory); err != nil {
		t.Fatal(err)
	}
	<-entered
	first := manager.Status("a")
	if first.State != "starting" || first.Invocation == "" {
		t.Fatalf("%+v", first)
	}
	results := make(chan error, 8)
	for range cap(results) {
		go func() { results <- manager.Start(ctx, "a", directory) }()
	}
	for range cap(results) {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if manager.Status("b").State != "stopped" {
		t.Fatal("crossed conversation state")
	}
	close(release)
	if err := manager.startAndWait(ctx, "a", directory); err != nil {
		t.Fatal(err)
	}
	process, err := manager.Process("a", first.Invocation)
	if err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 1 || process.command.Dir != directory {
		t.Fatal("duplicate or misdirected launch")
	}
	if err := manager.Stop(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if manager.Status("a").State != "stopped" {
		t.Fatal("conversation did not stop")
	}
	if err := manager.startAndWait(ctx, "a", directory); err != nil {
		t.Fatalf("restart after stop: %v", err)
	}
	if manager.Status("a").Invocation == first.Invocation || launches.Load() != 2 {
		t.Fatal("restart did not create a new invocation")
	}
	if _, err := manager.Process("a", first.Invocation); err == nil {
		t.Fatal("old invocation resolved after restart")
	}
	if manager.Status("b").State != "stopped" {
		t.Fatal("closing A changed B")
	}
}

func TestExecutionReadsAndClosure(t *testing.T) {
	manager := newExecutionManager(func(context.Context, string) (*wrapperProcess, error) {
		return nil, exec.ErrNotFound
	})
	if manager.Status("unused").State != "stopped" {
		t.Fatal("unused conversation is not stopped")
	}
	if _, err := manager.Process("unused", "old"); err == nil {
		t.Fatal("resolved an unknown invocation")
	}
	if len(manager.states) != 0 {
		t.Fatal("reads registered conversation state")
	}
	if err := manager.Stop(t.Context(), "unused"); err != nil {
		t.Fatal(err)
	}
	if len(manager.states) != 0 {
		t.Fatal("stopping an unused conversation registered state")
	}
	if err := manager.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(t.Context(), "new", ""); err == nil {
		t.Fatal("registered after shutdown")
	}
}

func TestExecutionReplacementKeepsProcessIdentity(t *testing.T) {
	manager := newExecutionManager(func(context.Context, string) (*wrapperProcess, error) {
		done := make(chan struct{})
		close(done)
		return &wrapperProcess{done: done, result: &exitResult{}, cancel: func() {}}, nil
	})
	t.Cleanup(func() { _ = manager.Close(context.WithoutCancel(t.Context())) })
	if err := manager.startAndWait(t.Context(), "a", ""); err != nil {
		t.Fatal(err)
	}
	first := manager.Status("a").Invocation
	original, err := manager.Process("a", first)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.startAndWait(t.Context(), "a", ""); err != nil {
		t.Fatal(err)
	}
	second := manager.Status("a").Invocation
	replacement, err := manager.Process("a", second)
	if err != nil || first == second || original == replacement {
		t.Fatalf("replacement reused invocation identity: %v", err)
	}
	if _, err := manager.Process("a", first); err == nil {
		t.Fatal("old invocation resolved to its replacement")
	}
}

func TestExecutionRequestCancellationAndShutdown(t *testing.T) {
	entered := make(chan context.Context, 1)
	finished := make(chan struct{})
	manager := newExecutionManager(func(ctx context.Context, _ string) (*wrapperProcess, error) {
		entered <- ctx
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	})
	t.Cleanup(func() { _ = manager.Close(t.Context()) })
	request, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- manager.startAndWait(request, "a", "") }()
	launch := <-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait: %v", err)
	}
	if launch.Err() != nil {
		t.Fatal("request cancellation stopped execution")
	}
	if err := manager.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before startup cleanup")
	}
}

func TestExecutionStartupFailureRetainsOwnership(t *testing.T) {
	for _, test := range []struct {
		name, script string
		uncertain    bool
	}{
		{name: "missing executable"},
		{name: "confirmed setup failure", script: `echo '{"error":"harness not installed"}'`},
		{name: "invalid readiness", script: "echo invalid; exec sleep 30", uncertain: true},
		{name: "missing readiness", script: "exit 1", uncertain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var launches atomic.Int32
			manager := newExecutionManager(func(ctx context.Context, _ string) (*wrapperProcess, error) {
				if launches.Add(1) > 1 || test.script == "" {
					return nil, exec.ErrNotFound
				}
				return startWrapper(ctx, "", "sh", "-c", test.script)
			})
			t.Cleanup(func() { _ = manager.Close(t.Context()) })
			if err := manager.startAndWait(t.Context(), "a", ""); err == nil {
				t.Fatal("startup unexpectedly succeeded")
			}
			status := manager.Status("a")
			process, _ := manager.Process("a", status.Invocation)
			if (process != nil) != test.uncertain {
				t.Fatalf("retained process = %v, uncertain = %v", process != nil, test.uncertain)
			}
			if process != nil && process.command.ProcessState == nil {
				t.Fatal("startup returned before wrapper cleanup")
			}
			if err := manager.startAndWait(t.Context(), "a", ""); err == nil {
				t.Fatal("retry unexpectedly succeeded")
			}
			if (launches.Load() == 1) != test.uncertain {
				t.Fatal("retry did not respect uncertain ownership")
			}
			if err := manager.Close(t.Context()); (err != nil) != test.uncertain {
				t.Fatalf("cleanup permission = %v", err)
			}
		})
	}
}

func TestExecutionStopDuringStartup(t *testing.T) {
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var launches atomic.Int32
	manager := newExecutionManager(func(ctx context.Context, _ string) (*wrapperProcess, error) {
		if launches.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
		}
		return nil, exec.ErrNotFound
	})
	t.Cleanup(func() { _ = manager.Close(t.Context()) })
	if err := manager.Start(t.Context(), "a", ""); err != nil {
		t.Fatal(err)
	}
	<-entered
	results := make(chan error, 2)
	go func() { results <- manager.Stop(t.Context(), "a") }()
	<-canceled
	go func() { results <- manager.Stop(t.Context(), "a") }()
	status := manager.Status("a")
	startErr := manager.Start(t.Context(), "a", "")
	close(release)
	if status.State != "stopping" || startErr == nil {
		t.Fatal("allowed a start during cleanup")
	}
	for range cap(results) {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Start(t.Context(), "a", ""); err != nil {
		t.Fatalf("restart after cleanup: %v", err)
	}
}
