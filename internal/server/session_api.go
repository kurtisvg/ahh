package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/server/sessions"
)

// serveSessionAPI handles routes under /api/sessions.
func (s *Server) serveSessionAPI(w http.ResponseWriter, r *http.Request) {
	id, action, ok := splitSessionAPIPath(r.URL.Path)
	if !ok || action != "tty" {
		http.NotFound(w, r)
		return
	}

	s.serveTTY(w, r, id)
}

// serveTTY proxies browser websocket traffic to the session wrapper's PTY
// endpoint. The public Ahh API uses tty terminology; the wrapper still owns the
// current PTY transport internally.
func (s *Server) serveTTY(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sessionWrapper, sessionStatus, ok := s.sessions.LookupWrapper(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if sessionStatus == sessions.StatusExited || sessionWrapper == nil {
		http.Error(w, "session exited", http.StatusGone)
		return
	}

	s.sessions.Touch(id)

	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wrapperConn, _, err := websocket.Dial(ctx, "ws://"+sessionWrapper.Address()+"/pty", nil)
	if err != nil {
		_ = browserConn.Close(websocket.StatusTryAgainLater, "wrapper unavailable")
		return
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- copyWebSocketMessages(ctx, wrapperConn, browserConn)
	}()
	go func() {
		errCh <- copyWebSocketMessages(ctx, browserConn, wrapperConn)
	}()

	err = <-errCh
	cancel()

	closeStatus := websocket.StatusNormalClosure
	reason := ""
	if !isExpectedWebSocketClose(err) {
		closeStatus = websocket.StatusInternalError
		reason = "terminal proxy failed"
	}

	_ = browserConn.Close(closeStatus, reason)
	_ = wrapperConn.Close(closeStatus, reason)
	<-errCh
}

// splitSessionAPIPath extracts the session id and action from /sessions/{id}/{action}.
func splitSessionAPIPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/sessions/")
	if rest == path || rest == "" {
		return "", "", false
	}

	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" || action == "" || strings.Contains(action, "/") {
		return "", "", false
	}

	return id, action, true
}

// copyWebSocketMessages forwards websocket frames until one side closes.
func copyWebSocketMessages(ctx context.Context, dst, src *websocket.Conn) error {
	for {
		messageType, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		if err := dst.Write(ctx, messageType, data); err != nil {
			return err
		}
	}
}

// isExpectedWebSocketClose reports whether err is normal connection shutdown noise.
func isExpectedWebSocketClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}

	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
		return true
	default:
		return false
	}
}
