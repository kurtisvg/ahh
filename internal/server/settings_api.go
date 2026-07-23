package server

import (
	"errors"
	"net/http"

	"github.com/kurtisvg/ahh/internal/server/settings"
)

type updateSettingsRequest struct {
	AuthenticationMode settings.AuthenticationMode `json:"authentication_mode"`
}

type regenerateIdentityRequest struct {
	ConfirmFingerprint string `json:"confirm_fingerprint"`
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.settings.Get())
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid settings request")
		return
	}

	updated, err := s.settings.SetAuthenticationMode(req.AuthenticationMode)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "authentication_mode must be managed or ambient")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) regenerateSSHIdentity(w http.ResponseWriter, r *http.Request) {
	var req regenerateIdentityRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid ssh identity regeneration request")
		return
	}

	updated, err := s.settings.Regenerate(req.ConfirmFingerprint)
	if errors.Is(err, settings.ErrFingerprintConfirmation) {
		writeAPIError(w, http.StatusConflict, "ssh identity fingerprint confirmation does not match")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "regenerate ssh identity failed")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
