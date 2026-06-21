package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	StateStopped  = "stopped"
	StateStarting = "starting"
	StateReady    = "ready"
	StateFailed   = "failed"
)

// Status describes the Ahh server's current view of a supervised harness.
type Status struct {
	Harness   string `json:"harness"`
	State     string `json:"state"`
	Address   string `json:"address"`
	LastError string `json:"last_error,omitempty"`
}

// Supervisor starts and stops a local harness wrapper process.
type Supervisor struct {
	commandPath string
	extraArgs   []string
	env         []string
	harness     string
	host        string
	httpClient  *http.Client

	mu     sync.Mutex
	cmd    *exec.Cmd
	waitc  chan error
	status Status
}

// Options configures a local harness supervisor.
type Options struct {
	CommandPath string
	ExtraArgs   []string
	Env         []string
	Harness     string
	Host        string
	HTTPClient  *http.Client
}

// New creates a local harness supervisor.
func New(opts Options) *Supervisor {
	commandPath := opts.CommandPath
	if commandPath == "" {
		commandPath = os.Args[0]
	}
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Second}
	}
	return &Supervisor{
		commandPath: commandPath,
		extraArgs:   append([]string(nil), opts.ExtraArgs...),
		env:         append([]string(nil), opts.Env...),
		harness:     opts.Harness,
		host:        host,
		httpClient:  client,
		status: Status{
			Harness: opts.Harness,
			State:   StateStopped,
		},
	}
}

// Start launches the harness wrapper and waits for its readiness endpoint.
func (s *Supervisor) Start(ctx context.Context) error {
	port, err := reservePort(s.host)
	if err != nil {
		s.setStatus(StateFailed, "", err)
		return err
	}
	address := net.JoinHostPort(s.host, port)

	args := append([]string{}, s.extraArgs...)
	args = append(args, "run", s.harness, "--host", s.host, "--port", port)
	cmd := exec.Command(s.commandPath, args...)
	if len(s.env) > 0 {
		cmd.Env = append(os.Environ(), s.env...)
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	s.mu.Lock()
	if s.cmd != nil {
		s.mu.Unlock()
		return errors.New("harness process already started")
	}
	s.cmd = cmd
	s.status = Status{Harness: s.harness, State: StateStarting, Address: address}
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.setStatus(StateFailed, address, err)
		return fmt.Errorf("start wrapper: %w", err)
	}

	waitc := make(chan error, 1)
	s.mu.Lock()
	s.waitc = waitc
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		waitc <- err
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cmd = nil
		s.waitc = nil
		if s.status.State != StateStopped && err != nil {
			s.status.State = StateFailed
			s.status.LastError = err.Error()
		}
	}()

	if err := s.waitReady(ctx, address); err != nil {
		_ = s.Stop(context.Background())
		s.setStatus(StateFailed, address, err)
		return err
	}

	s.setStatus(StateReady, address, nil)
	return nil
}

// Stop terminates the supervised harness wrapper process.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	waitc := s.waitc
	s.status.State = StateStopped
	s.status.LastError = ""
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case err := <-waitc:
		return err
	}
}

// Status returns the latest harness status snapshot.
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Supervisor) waitReady(ctx context.Context, address string) error {
	url := "http://" + address + "/readyz"
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline.Done():
			return fmt.Errorf("wait for wrapper readiness: %w", deadline.Err())
		case <-tick.C:
			req, err := http.NewRequestWithContext(deadline, http.MethodGet, url, nil)
			if err != nil {
				return err
			}
			resp, err := s.httpClient.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func (s *Supervisor) setStatus(state, address string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = state
	s.status.Address = address
	if err != nil {
		s.status.LastError = err.Error()
	} else {
		s.status.LastError = ""
	}
}

func reservePort(host string) (string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = ln.Close()
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "", err
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", err
	}
	return port, nil
}
