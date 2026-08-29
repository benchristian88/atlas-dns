package api

import (
	"net/http"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/onboarding"
)

func (s *Server) handleOnboardingStatus(response http.ResponseWriter, request *http.Request) {
	status, err := s.onboarding.Status(request.Context(), request.URL.Query().Get("clusterId"))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, status)
}

func (s *Server) handleOnboardingProgress(response http.ResponseWriter, request *http.Request) {
	var input struct {
		onboarding.ProgressInput
		RecordVersion int `json:"recordVersion"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	status, err := s.onboarding.Progress(request.Context(), actor(request.Context()), request.PathValue("clusterId"), input.RecordVersion, input.ProgressInput)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, status)
}

func (s *Server) handleFinishOnboarding(response http.ResponseWriter, request *http.Request) {
	var input struct {
		RecordVersion int `json:"recordVersion"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	status, err := s.onboarding.Finish(request.Context(), actor(request.Context()), request.PathValue("clusterId"), input.RecordVersion)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, status)
}

func (s *Server) handleValidateNodeCandidate(response http.ResponseWriter, request *http.Request) {
	var input nodeInput
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if input.Credentials == nil {
		s.writeError(response, request, domain.Validation("credentials", "are required"))
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	customCAPEM := ""
	if input.CustomCAPEM != nil {
		customCAPEM = *input.CustomCAPEM
	}
	result, err := s.management.ValidateNodeCandidate(request.Context(), domain.CreateNodeInput{
		ClusterID: request.PathValue("clusterId"), Name: input.Name, BaseURL: input.BaseURL,
		CertificatePolicy: input.CertificatePolicy, CustomCAPEM: customCAPEM,
		Username: input.Credentials.Username, Password: input.Credentials.Password, Enabled: enabled,
	})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"version": result.Version, "compatibility": result.Compatibility,
		"onboardingCompatibility": adguard.OnboardingCompatibility(result.Version),
		"running":                 result.Running, "protectionEnabled": result.ProtectionEnabled,
		"protectionDisabledDurationMillis": result.ProtectionDisabledDurationMS, "latencyMs": result.LatencyMS,
	})
}
