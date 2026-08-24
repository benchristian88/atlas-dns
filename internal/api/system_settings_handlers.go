package api

import (
	"net/http"
)

func (s *Server) handleSystemSettings(response http.ResponseWriter, request *http.Request) {
	settings, err := s.settings.Get(request.Context())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, settings)
}

func (s *Server) handleUpdateSystemSettings(response http.ResponseWriter, request *http.Request) {
	var input struct {
		UpdateChecksEnabled           *bool  `json:"updateChecksEnabled"`
		RecordVersion                 int    `json:"recordVersion"`
		NodeHealthIntervalSeconds     *int64 `json:"nodeHealthIntervalSeconds"`
		StatisticsPollIntervalSeconds *int64 `json:"statisticsPollIntervalSeconds"`
		QueryLogCollectionEnabled     *bool  `json:"queryLogCollectionEnabled"`
		QueryLogPollIntervalSeconds   *int64 `json:"queryLogPollIntervalSeconds"`
		QueryLogRetentionSeconds      *int64 `json:"queryLogRetentionSeconds"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	current, err := s.settings.Get(request.Context())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	if input.UpdateChecksEnabled != nil {
		current.UpdateChecksEnabled = *input.UpdateChecksEnabled
	}
	if input.NodeHealthIntervalSeconds != nil {
		current.NodeHealthIntervalSeconds = *input.NodeHealthIntervalSeconds
	}
	if input.StatisticsPollIntervalSeconds != nil {
		current.StatisticsPollIntervalSeconds = *input.StatisticsPollIntervalSeconds
	}
	if input.QueryLogCollectionEnabled != nil {
		current.QueryLogCollectionEnabled = *input.QueryLogCollectionEnabled
	}
	if input.QueryLogPollIntervalSeconds != nil {
		current.QueryLogPollIntervalSeconds = *input.QueryLogPollIntervalSeconds
	}
	if input.QueryLogRetentionSeconds != nil {
		current.QueryLogRetentionSeconds = *input.QueryLogRetentionSeconds
	}
	settings, err := s.settings.Update(request.Context(), actor(request.Context()), current, input.RecordVersion)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, settings)
}
