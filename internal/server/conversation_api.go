package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/server/conversations"
)

type createConversationRequest struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
}

type conversationsResponse struct {
	Conversations []conversations.Metadata `json:"conversations"`
}

func (s *Server) listConversations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, conversationsResponse{
		Conversations: s.conversations.List(),
	})
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var req createConversationRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid conversation request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "conversation name is required")
		return
	}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		writeAPIError(w, http.StatusBadRequest, "conversation agent_id is required")
		return
	}

	conversation, err := s.conversations.Create(r.Context(), name, agentID)
	if err != nil {
		if errors.Is(err, conversations.ErrAgentRequired) || errors.Is(err, conversations.ErrAgentNotFound) {
			writeAPIError(w, http.StatusBadRequest, "conversation agent is invalid")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "create conversation failed")
		return
	}

	writeJSON(w, http.StatusCreated, conversation.Metadata())
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.conversations.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete conversation failed")
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// serveTTY proxies browser websocket traffic to the conversation wrapper's PTY
// endpoint. The public Ahh API uses tty terminology; the wrapper still owns the
// current PTY transport internally.
func (s *Server) serveTTY(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	conversation, ok := s.conversations.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conversationWrapper, err := conversation.Start(r.Context())
	if err != nil {
		_ = browserConn.Close(websocket.StatusTryAgainLater, "conversation unavailable")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wrapperConn, _, err := websocket.Dial(ctx, "ws://"+conversationWrapper.Address()+"/pty", nil)
	if err != nil {
		_ = browserConn.Close(websocket.StatusTryAgainLater, "wrapper unavailable")
		return
	}

	// A conversation is touched when a browser successfully connects to its TTY
	// stream. Individual terminal frames do not update activity; this keeps
	// "most recent" tied to conversation selection rather than terminal noise.
	if err := conversation.Touch(); err != nil {
		_ = browserConn.Close(websocket.StatusInternalError, "conversation metadata unavailable")
		_ = wrapperConn.Close(websocket.StatusInternalError, "conversation metadata unavailable")
		return
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- copyBrowserToWrapper(ctx, wrapperConn, browserConn)
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
) error {
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
