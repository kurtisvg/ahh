package server

import "net/http"

func (s *Server) apiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sessions/", s.serveSessionAPI)

	return mux
}
