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
		UpdateChecksEnabled             *bool   `json:"updateChecksEnabled"`
		RecordVersion                   int     `json:"recordVersion"`
		SessionDurationSeconds          *int64  `json:"sessionDurationSeconds"`
		NodeHealthIntervalSeconds       *int64  `json:"nodeHealthIntervalSeconds"`
		NodeRequestTimeoutSeconds       *int64  `json:"nodeRequestTimeoutSeconds"`
		StatisticsPollIntervalSeconds   *int64  `json:"statisticsPollIntervalSeconds"`
		QueryLogCollectionEnabled       *bool   `json:"queryLogCollectionEnabled"`
		QueryLogPollIntervalSeconds     *int64  `json:"queryLogPollIntervalSeconds"`
		QueryLogRetentionSeconds        *int64  `json:"queryLogRetentionSeconds"`
		LogLevel                        *string `json:"logLevel"`
		OperationalHistoryRetentionDays *int    `json:"operationalHistoryRetentionDays"`
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
	if input.SessionDurationSeconds != nil {
		current.SessionDurationSeconds = *input.SessionDurationSeconds
	}
	if input.NodeHealthIntervalSeconds != nil {
		current.NodeHealthIntervalSeconds = *input.NodeHealthIntervalSeconds
	}
	if input.NodeRequestTimeoutSeconds != nil {
		current.NodeRequestTimeoutSeconds = *input.NodeRequestTimeoutSeconds
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
	if input.LogLevel != nil {
		current.LogLevel = *input.LogLevel
	}
	if input.OperationalHistoryRetentionDays != nil {
		current.OperationalHistoryRetentionDays = *input.OperationalHistoryRetentionDays
	}
	settings, err := s.settings.Update(request.Context(), actor(request.Context()), current, input.RecordVersion)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, settings)
}

func (s *Server) handleClearOperationalHistory(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Confirmation string `json:"confirmation"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.settings.ClearOperationalHistory(request.Context(), actor(request.Context()), input.Confirmation)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}
