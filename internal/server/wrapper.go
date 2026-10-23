package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// wrapperProcess owns a wrap subprocess and its startup output reader.
type wrapperProcess struct {
	command *exec.Cmd
	url     *url.URL
	cancel  context.CancelFunc

	done chan struct{}
	err  error // Read after done closes.
}

// startWrapper waits for the wrapper's startup JSON before returning ownership.
// On failure it stops the child and releases its output pipe.
func startWrapper(ctx context.Context, executable string, args ...string) (*wrapperProcess, error) {
	ctx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(ctx, executable, args...)
	command.Cancel = func() error {
		return command.Process.Signal(syscall.SIGTERM)
	}
	command.WaitDelay = 10 * time.Second
	command.Stderr = os.Stderr

	// Own the pipe so Wait can close it even if a child inherits stdout.
	stdout, writer, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	command.Stdout = writer
	if err := command.Start(); err != nil {
		cancel()
		_ = stdout.Close()
		_ = writer.Close()
		return nil, fmt.Errorf("start wrapper: %w", err)
	}
	_ = writer.Close()

	p := &wrapperProcess{
		command: command,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	startup := make(chan []byte, 1)
	readDone := make(chan struct{})

	// Read the startup message, then drain stdout so the child cannot block on output.
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 4096)
		if scanner.Scan() {
			startup <- append([]byte(nil), scanner.Bytes()...)
		} else {
			startup <- nil
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()

	// Finish process and output cleanup before allowing Wait to return.
	go func() {
		p.err = command.Wait()
		_ = stdout.Close()
		<-readDone
		cancel()
		close(p.done)
	}()

	ready, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	select {
	case data := <-startup:
		var started struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
		}
		err = json.Unmarshal(data, &started)
		if err == nil {
			if !net.ParseIP(started.Address).IsLoopback() || started.Port < 1 || started.Port > 65535 {
				err = errors.New("wrapper did not report a loopback address and valid port")
			} else {
				p.url = &url.URL{
					Scheme: "http",
					Host:   net.JoinHostPort(started.Address, strconv.Itoa(started.Port)),
				}
			}
		}
	case <-ready.Done():
		err = ready.Err()
	}
	if err != nil {
		shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		_ = p.Shutdown(shutdown)
		_ = p.Wait()
		return nil, fmt.Errorf("wrapper startup: %w", err)
	}
	return p, nil
}

// Wait joins the subprocess and startup reader.
func (p *wrapperProcess) Wait() error {
	<-p.done
	return p.err
}

// Shutdown sends SIGTERM; WaitDelay bounds the child's grace period.
// An earlier ctx deadline forces termination. Call Wait to finish cleanup.
func (p *wrapperProcess) Shutdown(ctx context.Context) error {
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		_ = p.command.Process.Kill()
		return ctx.Err()
	}
}
