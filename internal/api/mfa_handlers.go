package api

import (
	"net/http"
)

func (s *Server) handleMFAVerify(response http.ResponseWriter, request *http.Request) {
	s.completeMFAChallenge(response, request, false)
}

func (s *Server) handleMFARecovery(response http.ResponseWriter, request *http.Request) {
	s.completeMFAChallenge(response, request, true)
}

func (s *Server) completeMFAChallenge(response http.ResponseWriter, request *http.Request, recovery bool) {
	var input struct {
		Challenge    string `json:"challenge"`
		Code         string `json:"code,omitempty"`
		RecoveryCode string `json:"recoveryCode,omitempty"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	code := input.Code
	if recovery {
		code = input.RecoveryCode
	}
	result, err := s.auth.CompleteMFAChallenge(request.Context(), input.Challenge, code, recovery,
		requestID(request.Context()), remoteIP(request), request.UserAgent())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	s.setAuthCookies(response, result)
	writeJSON(response, http.StatusOK, authResponse{User: safeUser(result.User), ExpiresAt: result.Session.ExpiresAt})
}

func (s *Server) handleMFACancel(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Challenge string `json:"challenge"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if err := s.auth.CancelMFAChallenge(request.Context(), input.Challenge); err != nil {
		s.writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMFAStatus(response http.ResponseWriter, request *http.Request) {
	status, err := s.auth.MFAStatus(request.Context(), authenticatedUser(request.Context()).ID)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, status)
}

func (s *Server) handleMFAEnrollmentStart(response http.ResponseWriter, request *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.auth.StartMFAEnrollment(request.Context(), authenticatedUser(request.Context()).ID, input.CurrentPassword)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *Server) handleMFAEnrollmentVerify(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.auth.VerifyMFAEnrollment(request.Context(), authenticatedUser(request.Context()).ID,
		authenticatedSession(request.Context()).ID, input.Code, requestID(request.Context()))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *Server) handleMFARecoveryRegenerate(response http.ResponseWriter, request *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		Code            string `json:"code"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.auth.RegenerateRecoveryCodes(request.Context(), authenticatedUser(request.Context()).ID,
		authenticatedSession(request.Context()).ID, input.CurrentPassword, input.Code, requestID(request.Context()))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *Server) handleMFADisable(response http.ResponseWriter, request *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		Code            string `json:"code"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if err := s.auth.DisableMFA(request.Context(), authenticatedUser(request.Context()).ID,
		authenticatedSession(request.Context()).ID, input.CurrentPassword, input.Code, requestID(request.Context())); err != nil {
		s.writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
