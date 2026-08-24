package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/version"
)

type userResponse struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"displayName"`
	Role        domain.UserRole `json:"role"`
}

type authResponse struct {
	User      userResponse `json:"user"`
	ExpiresAt time.Time    `json:"expiresAt"`
}

func safeUser(user domain.User) userResponse {
	return userResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role}
}

func (s *Server) handleHealth(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{"status": "ok", "version": version.Current().Version})
}

func (s *Server) handleReady(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := s.health.Ping(ctx); err != nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready", "checks": map[string]string{"database": "unavailable"},
		})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"status": "ready", "checks": map[string]string{"database": "ok"}})
}

func (s *Server) handleSetupStatus(response http.ResponseWriter, request *http.Request) {
	required, err := s.auth.SetupRequired(request.Context())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"setupRequired":  required,
		"publicBaseUrl":  s.publicBaseURL,
		"controllerTime": time.Now().UTC(),
		"secureCookies":  s.secureCookies,
		"checks": map[string]string{
			"database": "ok", "credentialEncryption": "ok", "sessionProtection": "ok",
		},
	})
}

func (s *Server) handleSetup(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.auth.Setup(request.Context(), input.Email, input.DisplayName, input.Password,
		requestID(request.Context()), remoteIP(request), request.UserAgent())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	s.setAuthCookies(response, result)
	writeJSON(response, http.StatusCreated, authResponse{User: safeUser(result.User), ExpiresAt: result.Session.ExpiresAt})
}

func (s *Server) handleLogin(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	result, err := s.auth.Login(request.Context(), input.Email, input.Password,
		requestID(request.Context()), remoteIP(request), request.UserAgent())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	s.setAuthCookies(response, result)
	writeJSON(response, http.StatusOK, authResponse{User: safeUser(result.User), ExpiresAt: result.Session.ExpiresAt})
}

func (s *Server) handleLogout(response http.ResponseWriter, request *http.Request) {
	if err := s.auth.Logout(request.Context(), authenticatedSession(request.Context()), authenticatedUser(request.Context()), requestID(request.Context())); err != nil {
		s.writeError(response, request, err)
		return
	}
	s.clearAuthCookies(response)
	response.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(response http.ResponseWriter, request *http.Request) {
	session := authenticatedSession(request.Context())
	writeJSON(response, http.StatusOK, authResponse{User: safeUser(authenticatedUser(request.Context())), ExpiresAt: session.ExpiresAt})
}

type clusterInput struct {
	Name                 string                      `json:"name"`
	Description          string                      `json:"description"`
	ReconciliationPolicy domain.ReconciliationPolicy `json:"reconciliationPolicy,omitempty"`
	Version              int                         `json:"version,omitempty"`
}

func (s *Server) handleListClusters(response http.ResponseWriter, request *http.Request) {
	clusters, err := s.management.ListClusters(request.Context())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": clusters})
}

func (s *Server) handleCreateCluster(response http.ResponseWriter, request *http.Request) {
	var input clusterInput
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	cluster, err := s.management.CreateCluster(request.Context(), actor(request.Context()), domain.CreateClusterInput{Name: input.Name, Description: input.Description, ReconciliationPolicy: input.ReconciliationPolicy})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("Location", "/api/v1/clusters/"+cluster.ID)
	response.Header().Set("ETag", entityTag(cluster.Version))
	writeJSON(response, http.StatusCreated, cluster)
}

func (s *Server) handleGetCluster(response http.ResponseWriter, request *http.Request) {
	cluster, err := s.management.Cluster(request.Context(), request.PathValue("clusterId"))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", entityTag(cluster.Version))
	writeJSON(response, http.StatusOK, cluster)
}

func (s *Server) handleUpdateCluster(response http.ResponseWriter, request *http.Request) {
	var input clusterInput
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	cluster, err := s.management.UpdateCluster(request.Context(), actor(request.Context()), request.PathValue("clusterId"), input.Version, domain.CreateClusterInput{Name: input.Name, Description: input.Description, ReconciliationPolicy: input.ReconciliationPolicy})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", entityTag(cluster.Version))
	writeJSON(response, http.StatusOK, cluster)
}

type credentialsInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type nodeInput struct {
	Name              string                   `json:"name"`
	BaseURL           string                   `json:"baseUrl"`
	CertificatePolicy domain.CertificatePolicy `json:"certificatePolicy"`
	CustomCAPEM       *string                  `json:"customCaPem,omitempty"`
	Credentials       *credentialsInput        `json:"credentials,omitempty"`
	Enabled           *bool                    `json:"enabled,omitempty"`
	RecordVersion     int                      `json:"recordVersion,omitempty"`
}

func (s *Server) handleListNodes(response http.ResponseWriter, request *http.Request) {
	nodes, err := s.management.ListNodes(request.Context(), request.PathValue("clusterId"))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	healthInterval := s.healthInterval
	if s.runtime != nil {
		healthInterval = s.runtime.RuntimeSettings().NodeHealthInterval
	}
	staleAfterSeconds := max(int64(healthInterval/time.Second)*3, 1)
	writeJSON(response, http.StatusOK, map[string]any{
		"items": nodes, "refreshedAt": time.Now().UTC(), "staleAfterSeconds": staleAfterSeconds,
	})
}

func (s *Server) handleCreateNode(response http.ResponseWriter, request *http.Request) {
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
	node, err := s.management.CreateNode(request.Context(), actor(request.Context()), domain.CreateNodeInput{
		ClusterID: request.PathValue("clusterId"), Name: input.Name, BaseURL: input.BaseURL,
		CertificatePolicy: input.CertificatePolicy, CustomCAPEM: customCAPEM,
		Username: input.Credentials.Username, Password: input.Credentials.Password, Enabled: enabled,
	})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("Location", "/api/v1/nodes/"+node.ID)
	response.Header().Set("ETag", entityTag(node.RecordVersion))
	writeJSON(response, http.StatusCreated, node)
}

func (s *Server) handleGetNode(response http.ResponseWriter, request *http.Request) {
	node, err := s.management.Node(request.Context(), request.PathValue("nodeId"))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", entityTag(node.RecordVersion))
	writeJSON(response, http.StatusOK, node)
}

func (s *Server) handleUpdateNode(response http.ResponseWriter, request *http.Request) {
	var input nodeInput
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	var username, password *string
	if input.Credentials != nil {
		username = &input.Credentials.Username
		password = &input.Credentials.Password
	}
	node, err := s.management.UpdateNode(request.Context(), actor(request.Context()), request.PathValue("nodeId"), domain.UpdateNodeInput{
		Name: input.Name, BaseURL: input.BaseURL, CertificatePolicy: input.CertificatePolicy,
		CustomCAPEM: input.CustomCAPEM, Username: username, Password: password,
		Enabled: enabled, ExpectedVersion: input.RecordVersion,
	})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", entityTag(node.RecordVersion))
	writeJSON(response, http.StatusOK, node)
}

func (s *Server) handleDeleteNode(response http.ResponseWriter, request *http.Request) {
	var input struct {
		RecordVersion int    `json:"recordVersion"`
		ConfirmName   string `json:"confirmName"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if err := s.management.DeleteNode(request.Context(), actor(request.Context()), request.PathValue("nodeId"), input.ConfirmName, input.RecordVersion); err != nil {
		s.writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestNode(response http.ResponseWriter, request *http.Request) {
	result, err := s.management.TestNodeConnection(request.Context(), actor(request.Context()), request.PathValue("nodeId"))
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *Server) handleNodeMaintenance(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Enabled       bool   `json:"enabled"`
		RecordVersion int    `json:"recordVersion"`
		BreakGlass    bool   `json:"breakGlass"`
		Confirmation  string `json:"confirmation"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	var node domain.Node
	var err error
	if s.haOperations != nil && input.Enabled {
		node, err = s.haOperations.EnterMaintenance(request.Context(), actor(request.Context()), request.PathValue("nodeId"), input.RecordVersion, input.BreakGlass, input.Confirmation)
	} else if s.haOperations != nil {
		_, err = s.haOperations.ReturnToService(request.Context(), actor(request.Context()), request.PathValue("nodeId"), input.RecordVersion)
		if err == nil {
			node, err = s.management.Node(request.Context(), request.PathValue("nodeId"))
		}
	} else {
		node, err = s.management.SetNodeMaintenance(request.Context(), actor(request.Context()), request.PathValue("nodeId"), input.Enabled, input.RecordVersion)
	}
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", entityTag(node.RecordVersion))
	writeJSON(response, http.StatusOK, node)
}

func (s *Server) handleAuditEvents(response http.ResponseWriter, request *http.Request) {
	limit := parseBoundedInt(request.URL.Query().Get("limit"), 50, 1, 100)
	offset := parseBoundedInt(request.URL.Query().Get("offset"), 0, 0, 100000)
	events, err := s.audit.ListAuditEvents(request.Context(), limit, offset)
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": events, "limit": limit, "offset": offset})
}

func (s *Server) handleVersion(response http.ResponseWriter, request *http.Request) {
	info := version.Current()
	schemaVersion := int64(0)
	if reader, ok := s.health.(interface {
		CurrentSchemaVersion(context.Context) (int64, error)
	}); ok {
		current, err := reader.CurrentSchemaVersion(request.Context())
		if err != nil {
			s.writeError(response, request, err)
			return
		}
		schemaVersion = current
	}
	writeJSON(response, http.StatusOK, map[string]any{"version": info.Version, "commit": info.Commit, "builtAt": info.BuiltAt, "development": info.Development, "databaseSchemaVersion": schemaVersion})
}

func entityTag(value int) string {
	return `"` + strconv.Itoa(value) + `"`
}

func parseBoundedInt(value string, fallback, minimum, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < minimum || parsed > maximum {
		return fallback
	}
	return parsed
}
