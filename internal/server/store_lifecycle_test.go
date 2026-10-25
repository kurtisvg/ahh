package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/project"
)

type lifecycleStore struct {
	testProjects
	entered, release, closed chan struct{}
	closeErr                 error
	closes                   atomic.Int32
}

func (p *lifecycleStore) ListProjects(context.Context) ([]project.Project, error) {
	close(p.entered)
	<-p.release
	return []project.Project{}, nil
}

func (p *lifecycleStore) Close() error {
	if p.closes.Add(1) == 1 {
		close(p.closed)
	}
	return p.closeErr
}

func TestStartFailureClosesStore(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		if canceled {
			cancel()
		}
		closeErr := errors.New("close failed")
		p := &lifecycleStore{closed: make(chan struct{}), closeErr: closeErr}
		path := t.TempDir()
		_, err := startWithStore(t, ctx, Config{Address: "127.0.0.1", Port: -1, Workspace: path}, p, "unused")
		cancel()
		if !errors.Is(err, closeErr) || p.closes.Load() != 1 {
			t.Fatalf("startup error = %v, close calls = %d", err, p.closes.Load())
		}
		if canceled && !errors.Is(err, context.Canceled) {
			t.Fatalf("lost startup cancellation: %v", err)
		}
		lock, err := lockWorkspace(path)
		if err != nil {
			t.Fatalf("startup failure retained workspace lock: %v", err)
		}
		_ = lock.Close()
	}
}

func TestWaitClosesStoreAfterRequests(t *testing.T) {
	p := &lifecycleStore{
		entered: make(chan struct{}), release: make(chan struct{}),
		closed: make(chan struct{}), closeErr: errors.New("close failed"),
	}
	s, err := startWithStore(t, t.Context(), Config{Address: "127.0.0.1"}, p, "unused")
	if err != nil {
		t.Fatal(err)
	}
	shutdown, cancel := context.WithCancel(t.Context())
	cancel() // Force shutdown without waiting for HTTP handlers to drain.
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
		_ = s.Shutdown(shutdown)
		_ = s.Wait()
	})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.http.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	}()
	<-p.entered
	_ = s.Shutdown(shutdown)
	select {
	case <-p.closed:
		t.Fatal("closed storage while a request was still using it")
	case <-time.After(20 * time.Millisecond):
	}
	if lock, err := lockWorkspace(s.Workspace); err == nil {
		_ = lock.Close()
		t.Fatal("released workspace while a request was still using storage")
	}
	close(p.release)
	<-finished
	if err := s.Wait(); !errors.Is(err, p.closeErr) {
		t.Fatalf("lost storage cleanup error: %v", err)
	}
	_ = s.Shutdown(shutdown)
	_ = s.Wait()
	if p.closes.Load() != 1 {
		t.Fatalf("close calls = %d", p.closes.Load())
	}
	lock, err := lockWorkspace(s.Workspace)
	if err != nil {
		t.Fatalf("Wait retained workspace lock: %v", err)
	}
	_ = lock.Close()
}
