package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/server/sessions"
)

type createSessionRequest struct {
	Name string `json:"name"`
}

type sessionsResponse struct {
	Sessions []sessions.Metadata `json:"sessions"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Server) listSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, sessionsResponse{
		Sessions: s.sessions.List(),
	})
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid session request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "session name is required")
		return
	}

	session, err := s.sessions.Create(r.Context(), name)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "create session failed")
		return
	}

	writeJSON(w, http.StatusCreated, session.Metadata())
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.sessions.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete session failed")
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{
		Error: message,
	})
}

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

	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	sessionWrapper, err := session.Start(r.Context())
	if err != nil {
		_ = browserConn.Close(websocket.StatusTryAgainLater, "session unavailable")
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
	if err := session.Touch(); err != nil {
		_ = browserConn.Close(websocket.StatusInternalError, "session metadata unavailable")
		_ = wrapperConn.Close(websocket.StatusInternalError, "session metadata unavailable")
		return
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- copyBrowserToWrapper(ctx, wrapperConn, browserConn, session)
	}()
	go func() {
		errCh <- copyWrapperToBrowser(ctx, browserConn, wrapperConn)
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

func copyBrowserToWrapper(
	ctx context.Context,
	dst *websocket.Conn,
	src *websocket.Conn,
	session *sessions.Session,
) error {
	for {
		messageType, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		if err := dst.Write(ctx, messageType, data); err != nil {
			return err
		}
		if submittedTerminalInput(messageType, data) {
			if err := session.MarkResumable(); err != nil {
				return err
			}
		}
	}
}

func submittedTerminalInput(messageType websocket.MessageType, data []byte) bool {
	if messageType != websocket.MessageText {
		return false
	}

	var msg struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return false
	}

	return msg.Type == "input" && strings.ContainsAny(msg.Data, "\r\n")
}

// copyWrapperToBrowser forwards wrapper PTY output to the browser until one side closes.
func copyWrapperToBrowser(ctx context.Context, dst, src *websocket.Conn) error {
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
