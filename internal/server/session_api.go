package server

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/server/sessions"
)

// serveTTY proxies browser websocket traffic to the session wrapper's PTY
// endpoint. The public Ahh API uses tty terminology; the wrapper still owns the
// current PTY transport internally.
func (s *Server) serveTTY(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	session, ok := s.sessions.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	sessionWrapper, sessionStatus := session.Wrapper()
	if sessionStatus == sessions.StatusExited || sessionWrapper == nil {
		http.Error(w, "session exited", http.StatusGone)
		return
	}

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

	// A session is touched when a browser successfully connects to its TTY
	// stream. Individual terminal frames do not update activity; this keeps
	// "most recent" tied to session selection rather than terminal noise.
	session.Touch()

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
