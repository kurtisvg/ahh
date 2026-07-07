package server

import "net/http"

// apiHandler owns routes under /api after the server strips that prefix.
func (s *Server) apiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sessions/", s.serveSessionAPI)

	return mux
}
