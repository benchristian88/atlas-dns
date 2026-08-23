package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/backup"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
	"github.com/benchristian88/atlas-dns/internal/operations"
	"github.com/benchristian88/atlas-dns/internal/querylog"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
	"github.com/benchristian88/atlas-dns/internal/updates"
	"github.com/benchristian88/atlas-dns/internal/useradmin"
)

const (
	sessionCookieName = "atlas_dns_session"
	csrfCookieName    = "atlas_dns_csrf"
	requestIDHeader   = "X-Request-ID"
	csrfHeader        = "X-CSRF-Token"
	idempotencyHeader = "Idempotency-Key"
)

type AuditReader interface {
	ListAuditEvents(context.Context, int, int) ([]domain.AuditEvent, error)
}

type HealthChecker interface {
	Ping(context.Context) error
}

type BlockedServicesCatalogueReader interface {
	BlockedServicesCatalogue(context.Context, string) (inventory.BlockedServicesCatalogue, error)
}

type BlocklistPresentationReader interface {
	BlocklistPresentation(context.Context, string) (inventory.BlocklistPresentation, error)
}

type AllowlistPresentationReader interface {
	AllowlistPresentation(context.Context, string) (inventory.AllowlistPresentation, error)
}

type DHCPInterfacesReader interface {
	DHCPInterfaces(context.Context, string) (inventory.DHCPInterfaces, error)
}

type DHCPActiveChecker interface {
	FindActiveDHCP(context.Context, domain.Actor, string, string) (inventory.DHCPActiveCheckResult, error)
}

type DHCPOperationService interface {
	RunDHCPOperation(context.Context, domain.Actor, string, inventory.DHCPOperationCommand, string, string) (inventory.DHCPOperation, error)
	ListDHCPOperations(context.Context, string, int) ([]inventory.DHCPOperation, error)
}

type DNSOperationService interface {
	StartUpstreamTest(context.Context, domain.Actor, string, operations.Target, operations.UpstreamInput, string) (operations.Operation, error)
	StartHostFilterTest(context.Context, domain.Actor, string, operations.Target, operations.HostFilterInput, string) (operations.Operation, error)
	StartCacheClear(context.Context, domain.Actor, string, operations.Target, string, string) (operations.Operation, error)
	StartQueryLogClear(context.Context, domain.Actor, string, operations.Target, string, string) (operations.Operation, error)
	StartStatisticsReset(context.Context, domain.Actor, string, operations.Target, string, string) (operations.Operation, error)
	Operation(context.Context, string) (operations.Operation, error)
	List(context.Context, string, operations.Command, int) ([]operations.Operation, error)
}

type StatisticsService interface {
	Statistics(context.Context, string, telemetry.Range, string, int) (telemetry.Report, error)
}

type QueryLogService interface {
	List(context.Context, querylog.ListRequest) (querylog.Page, error)
	Detail(context.Context, string, string) (querylog.Event, error)
}

type OperationalHealthService interface {
	Status(context.Context, string) (operationalhealth.Status, error)
}

type HAOperationsService interface {
	Summary(context.Context, string) (haoperations.HASummary, error)
	Lifecycle(context.Context, string) (haoperations.NodeLifecycle, error)
	Settings(context.Context, string) (haoperations.NodeSettings, error)
	UpdateSettings(context.Context, domain.Actor, string, haoperations.NodeSettings, int) (haoperations.NodeSettings, error)
	ProbeNode(context.Context, string) (haoperations.DNSProbeResult, error)
	Certificates(context.Context, string) ([]haoperations.Certificate, error)
	Versions(context.Context, string) ([]haoperations.VersionState, error)
	MaintenancePreflight(context.Context, string) (haoperations.MaintenancePreflight, error)
	EnterMaintenance(context.Context, domain.Actor, string, int, bool, string) (domain.Node, error)
	ReturnToService(context.Context, domain.Actor, string, int) (haoperations.ReturnValidation, error)
	History(context.Context, haoperations.HistoryRequest) (haoperations.HistoryPage, error)
	StartUpgrade(context.Context, domain.Actor, string, string) (haoperations.Upgrade, error)
	CompleteUpgrade(context.Context, domain.Actor, string, int) (haoperations.Upgrade, error)
	Upgrades(context.Context, string, int) ([]haoperations.Upgrade, error)
}

type NotificationSettingsService interface {
	List(context.Context, string) ([]haoperations.NotificationChannel, error)
	Create(context.Context, domain.Actor, string, string, string, bool) (haoperations.NotificationChannel, error)
	Update(context.Context, domain.Actor, string, string, *string, bool, int) (haoperations.NotificationChannel, error)
	Delete(context.Context, domain.Actor, string, string, int) error
	Test(context.Context, domain.Actor, string) (haoperations.NotificationTestResult, error)
}

type UserAdministrationService interface {
	List(context.Context) ([]domain.User, error)
	Create(context.Context, domain.Actor, useradmin.CreateInput) (domain.User, error)
	Update(context.Context, domain.Actor, string, useradmin.UpdateInput) (domain.User, error)
	ResetPassword(context.Context, domain.Actor, string, string) error
}

type BackupService interface {
	Create(context.Context, backup.Type, string, domain.Actor) (backup.Result, error)
}

type ControllerUpdateService interface {
	Status(context.Context, bool) (updates.Status, error)
}

type SystemSettingsService interface {
	Get(context.Context) (systemsettings.Settings, error)
	Update(context.Context, domain.Actor, bool, int) (systemsettings.Settings, error)
}

type Server struct {
	auth           *auth.Service
	management     *domain.ManagementService
	inventory      *inventory.Service
	catalogue      BlockedServicesCatalogueReader
	blocklists     BlocklistPresentationReader
	allowlists     AllowlistPresentationReader
	dhcpInterfaces DHCPInterfacesReader
	dhcpChecker    DHCPActiveChecker
	dhcpOperations DHCPOperationService
	dnsOperations  DNSOperationService
	statistics     StatisticsService
	queryLog       QueryLogService
	operational    OperationalHealthService
	haOperations   HAOperationsService
	notifications  NotificationSettingsService
	users          UserAdministrationService
	backups        BackupService
	updates        ControllerUpdateService
	settings       SystemSettingsService
	metrics        *operationalhealth.Tracker
	metricsToken   string
	controlplane   *controlplane.Service
	audit          AuditReader
	health         HealthChecker
	logger         *slog.Logger
	secureCookies  bool
	publicBaseURL  string
	healthInterval time.Duration
	webDist        string
	mux            *http.ServeMux
}

func (s *Server) SetDNSOperations(service DNSOperationService)          { s.dnsOperations = service }
func (s *Server) SetStatistics(service StatisticsService)               { s.statistics = service }
func (s *Server) SetQueryLog(service QueryLogService)                   { s.queryLog = service }
func (s *Server) SetOperationalHealth(service OperationalHealthService) { s.operational = service }
func (s *Server) SetHAOperations(service HAOperationsService)           { s.haOperations = service }
func (s *Server) SetNotificationSettings(service NotificationSettingsService) {
	s.notifications = service
}
func (s *Server) SetUserAdministration(service UserAdministrationService) { s.users = service }
func (s *Server) SetBackups(service BackupService)                        { s.backups = service }
func (s *Server) SetControllerUpdates(service ControllerUpdateService)    { s.updates = service }
func (s *Server) SetSystemSettings(service SystemSettingsService)         { s.settings = service }
func (s *Server) SetMetrics(tracker *operationalhealth.Tracker, token string) {
	s.metrics, s.metricsToken = tracker, token
}

func NewServer(authService *auth.Service, management *domain.ManagementService, inventoryService *inventory.Service, audit AuditReader, health HealthChecker, logger *slog.Logger, secureCookies bool, publicBaseURL string, healthInterval time.Duration, webDist string, controlplanes ...*controlplane.Service) *Server {
	var controlplaneService *controlplane.Service
	if len(controlplanes) > 0 {
		controlplaneService = controlplanes[0]
	}
	server := &Server{
		auth: authService, management: management, inventory: inventoryService, catalogue: inventoryService, blocklists: inventoryService, allowlists: inventoryService, dhcpInterfaces: inventoryService, dhcpChecker: inventoryService, dhcpOperations: inventoryService, audit: audit, health: health,
		controlplane: controlplaneService,
		logger:       logger, secureCookies: secureCookies, publicBaseURL: publicBaseURL, healthInterval: healthInterval,
		webDist: webDist, mux: http.NewServeMux(),
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.requestID(s.recover(s.accessLog(s.mux))))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ready", s.handleReady)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /api/v1/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/v1/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.Handle("POST /api/v1/auth/logout", s.authenticated(true, http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("GET /api/v1/auth/me", s.authenticated(false, http.HandlerFunc(s.handleMe)))
	s.mux.Handle("GET /api/v1/users", s.administrator(false, http.HandlerFunc(s.handleListUsers)))
	s.mux.Handle("POST /api/v1/users", s.administrator(true, http.HandlerFunc(s.handleCreateUser)))
	s.mux.Handle("PATCH /api/v1/users/{userId}", s.administrator(true, http.HandlerFunc(s.handleUpdateUser)))
	s.mux.Handle("POST /api/v1/users/{userId}/password-reset", s.administrator(true, http.HandlerFunc(s.handleResetUserPassword)))
	s.mux.Handle("POST /api/v1/system/backups", s.administrator(true, http.HandlerFunc(s.handleCreateBackup)))
	s.mux.Handle("POST /api/v1/system/restore-preflight", s.administrator(true, http.HandlerFunc(s.handleRestorePreflight)))
	s.mux.Handle("GET /api/v1/system/update", s.administrator(false, http.HandlerFunc(s.handleControllerUpdate)))
	s.mux.Handle("POST /api/v1/system/update/check", s.administrator(true, http.HandlerFunc(s.handleCheckControllerUpdate)))
	s.mux.Handle("GET /api/v1/system/settings", s.administrator(false, http.HandlerFunc(s.handleSystemSettings)))
	s.mux.Handle("PATCH /api/v1/system/settings", s.administrator(true, http.HandlerFunc(s.handleUpdateSystemSettings)))
	s.mux.Handle("GET /api/v1/clusters", s.authenticated(false, http.HandlerFunc(s.handleListClusters)))
	s.mux.Handle("POST /api/v1/clusters", s.authenticated(true, http.HandlerFunc(s.handleCreateCluster)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}", s.authenticated(false, http.HandlerFunc(s.handleGetCluster)))
	s.mux.Handle("PATCH /api/v1/clusters/{clusterId}", s.authenticated(true, http.HandlerFunc(s.handleUpdateCluster)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/nodes", s.authenticated(false, http.HandlerFunc(s.handleListNodes)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/nodes", s.authenticated(true, http.HandlerFunc(s.handleCreateNode)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/statistics", s.authenticated(false, http.HandlerFunc(s.handleStatistics)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/query-events", s.authenticated(false, http.HandlerFunc(s.handleQueryEvents)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/query-events/{eventId}", s.authenticated(false, http.HandlerFunc(s.handleQueryEventDetail)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/operational-status", s.authenticated(false, http.HandlerFunc(s.handleOperationalStatus)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/ha-status", s.authenticated(false, http.HandlerFunc(s.handleHAStatus)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/ha-history", s.authenticated(false, http.HandlerFunc(s.handleHAHistory)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/certificates", s.authenticated(false, http.HandlerFunc(s.handleCertificates)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/versions", s.authenticated(false, http.HandlerFunc(s.handleVersions)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/upgrades", s.authenticated(false, http.HandlerFunc(s.handleUpgrades)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/notification-channels", s.authenticated(false, http.HandlerFunc(s.handleNotificationChannels)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/notification-channels", s.administrator(true, http.HandlerFunc(s.handleCreateNotificationChannel)))
	s.mux.Handle("GET /api/v1/nodes/{nodeId}", s.authenticated(false, http.HandlerFunc(s.handleGetNode)))
	s.mux.Handle("GET /api/v1/nodes/{nodeId}/lifecycle", s.authenticated(false, http.HandlerFunc(s.handleNodeLifecycle)))
	s.mux.Handle("PUT /api/v1/nodes/{nodeId}/lifecycle-settings", s.authenticated(true, http.HandlerFunc(s.handleLifecycleSettings)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/dns-probe", s.authenticated(true, http.HandlerFunc(s.handleDNSProbe)))
	s.mux.Handle("GET /api/v1/nodes/{nodeId}/maintenance-preflight", s.authenticated(false, http.HandlerFunc(s.handleMaintenancePreflight)))
	s.mux.Handle("PATCH /api/v1/nodes/{nodeId}", s.authenticated(true, http.HandlerFunc(s.handleUpdateNode)))
	s.mux.Handle("DELETE /api/v1/nodes/{nodeId}", s.authenticated(true, http.HandlerFunc(s.handleDeleteNode)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/test-connection", s.authenticated(true, http.HandlerFunc(s.handleTestNode)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/maintenance", s.authenticated(true, http.HandlerFunc(s.handleNodeMaintenance)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/return-to-service", s.authenticated(true, http.HandlerFunc(s.handleReturnToService)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/upgrades", s.authenticated(true, http.HandlerFunc(s.handleStartUpgrade)))
	s.mux.Handle("POST /api/v1/upgrades/{upgradeId}/validate", s.authenticated(true, http.HandlerFunc(s.handleValidateUpgrade)))
	s.mux.Handle("PATCH /api/v1/notification-channels/{channelId}", s.administrator(true, http.HandlerFunc(s.handleUpdateNotificationChannel)))
	s.mux.Handle("POST /api/v1/notification-channels/{channelId}/test", s.administrator(true, http.HandlerFunc(s.handleTestNotificationChannel)))
	s.mux.Handle("DELETE /api/v1/notification-channels/{channelId}", s.administrator(true, http.HandlerFunc(s.handleDeleteNotificationChannel)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/observations", s.authenticated(true, http.HandlerFunc(s.handleObserveNode)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/filter-refresh", s.authenticated(true, http.HandlerFunc(s.handleFilterRefresh)))
	s.mux.Handle("GET /api/v1/nodes/{nodeId}/dhcp/interfaces", s.authenticated(false, http.HandlerFunc(s.handleDHCPInterfaces)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/dhcp/active-check", s.authenticated(true, http.HandlerFunc(s.handleDHCPActiveCheck)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/dhcp/reset-leases", s.authenticated(true, http.HandlerFunc(s.handleDHCPResetLeases)))
	s.mux.Handle("POST /api/v1/nodes/{nodeId}/dhcp/reset-configuration", s.authenticated(true, http.HandlerFunc(s.handleDHCPResetConfiguration)))
	s.mux.Handle("GET /api/v1/nodes/{nodeId}/dhcp/operations", s.authenticated(false, http.HandlerFunc(s.handleListDHCPOperations)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/operational-commands/test-upstream-dns", s.authenticated(true, http.HandlerFunc(s.handleTestUpstreamDNS)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/operational-commands/test-host-filtering", s.authenticated(true, http.HandlerFunc(s.handleTestHostFiltering)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/operational-commands/clear-dns-cache", s.authenticated(true, http.HandlerFunc(s.handleClearDNSCache)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/operational-commands/clear-query-log", s.authenticated(true, http.HandlerFunc(s.handleClearQueryLog)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/operational-commands/reset-statistics", s.authenticated(true, http.HandlerFunc(s.handleResetStatistics)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/operational-commands", s.authenticated(false, http.HandlerFunc(s.handleListDNSOperations)))
	s.mux.Handle("GET /api/v1/operational-commands/{operationId}", s.authenticated(false, http.HandlerFunc(s.handleGetDNSOperation)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/configuration-inventory", s.authenticated(false, http.HandlerFunc(s.handleConfigurationInventory)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/blocklists/presentation", s.authenticated(false, http.HandlerFunc(s.handleBlocklistPresentation)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/allowlists/presentation", s.authenticated(false, http.HandlerFunc(s.handleAllowlistPresentation)))
	s.mux.Handle("GET /api/v1/clusters/{clusterId}/blocked-services/catalogue", s.authenticated(false, http.HandlerFunc(s.handleBlockedServicesCatalogue)))
	s.mux.Handle("GET /api/v1/configuration-comparisons", s.authenticated(false, http.HandlerFunc(s.handleConfigurationComparison)))
	s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-draft/import", s.authenticated(true, http.HandlerFunc(s.handleImportConfiguration)))
	if s.controlplane != nil {
		s.mux.Handle("PUT /api/v1/clusters/{clusterId}/configuration-draft", s.authenticated(true, http.HandlerFunc(s.handleUpdateConfigurationDraft)))
		s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-draft/validate", s.authenticated(true, http.HandlerFunc(s.handleValidateConfigurationDraft)))
		s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-revisions", s.authenticated(true, http.HandlerFunc(s.handlePublishConfigurationRevision)))
		s.mux.Handle("GET /api/v1/clusters/{clusterId}/configuration-revisions", s.authenticated(false, http.HandlerFunc(s.handleListConfigurationRevisions)))
		s.mux.Handle("GET /api/v1/configuration-revisions/{revisionId}", s.authenticated(false, http.HandlerFunc(s.handleGetConfigurationRevision)))
		s.mux.Handle("POST /api/v1/configuration-revisions/{revisionId}/archive", s.administrator(true, http.HandlerFunc(s.handleArchiveConfigurationRevision)))
		s.mux.Handle("POST /api/v1/configuration-revisions/{revisionId}/restore", s.administrator(true, http.HandlerFunc(s.handleRestoreConfigurationRevision)))
		s.mux.Handle("DELETE /api/v1/configuration-revisions/{revisionId}", s.administrator(true, http.HandlerFunc(s.handleDeleteConfigurationRevision)))
		s.mux.Handle("GET /api/v1/configuration-revision-comparisons", s.authenticated(false, http.HandlerFunc(s.handleConfigurationRevisionComparison)))
		s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/deployment-preview", s.authenticated(true, http.HandlerFunc(s.handleDeploymentPreview)))
		s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/deployments", s.authenticated(true, http.HandlerFunc(s.handleStartDeployment)))
		s.mux.Handle("POST /api/v1/clusters/{clusterId}/configuration-revisions/{revisionId}/rollback", s.authenticated(true, http.HandlerFunc(s.handleRollback)))
		s.mux.Handle("GET /api/v1/clusters/{clusterId}/deployments", s.authenticated(false, http.HandlerFunc(s.handleListDeployments)))
		s.mux.Handle("GET /api/v1/deployments/{deploymentId}", s.authenticated(false, http.HandlerFunc(s.handleGetDeployment)))
		s.mux.Handle("POST /api/v1/deployments/{deploymentId}/cancel", s.authenticated(true, http.HandlerFunc(s.handleCancelDeployment)))
		s.mux.Handle("POST /api/v1/deployments/{deploymentId}/archive", s.administrator(true, http.HandlerFunc(s.handleArchiveDeployment)))
		s.mux.Handle("POST /api/v1/deployments/{deploymentId}/restore", s.administrator(true, http.HandlerFunc(s.handleRestoreDeployment)))
		s.mux.Handle("DELETE /api/v1/deployments/{deploymentId}", s.administrator(true, http.HandlerFunc(s.handleDeleteDeployment)))
		s.mux.Handle("GET /api/v1/clusters/{clusterId}/drift-events", s.authenticated(false, http.HandlerFunc(s.handleListDriftEvents)))
		s.mux.Handle("POST /api/v1/drift-events/{driftId}/restore", s.authenticated(true, http.HandlerFunc(s.handleRestoreDrift)))
		s.mux.Handle("POST /api/v1/drift-events/{driftId}/adopt", s.authenticated(true, http.HandlerFunc(s.handleAdoptDrift)))
	}
	s.mux.Handle("GET /api/v1/audit-events", s.authenticated(false, http.HandlerFunc(s.handleAuditEvents)))
	s.mux.Handle("GET /api/v1/system/version", s.authenticated(false, http.HandlerFunc(s.handleVersion)))
	s.mux.HandleFunc("/", s.handleFrontend)
}

type contextKey string

const (
	requestIDKey contextKey = "request-id"
	sessionKey   contextKey = "session"
	userKey      contextKey = "user"
)

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func authenticatedUser(ctx context.Context) domain.User {
	value, _ := ctx.Value(userKey).(domain.User)
	return value
}

func authenticatedSession(ctx context.Context) domain.Session {
	value, _ := ctx.Value(sessionKey).(domain.Session)
	return value
}

func actor(ctx context.Context) domain.Actor {
	return domain.Actor{UserID: authenticatedUser(ctx).ID, RequestID: requestID(ctx)}
}

func (s *Server) authenticated(requireCSRF bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil {
			s.writeError(response, request, domain.NewError(domain.ErrorAuthentication, "authentication is required"))
			return
		}
		session, user, err := s.auth.Authenticate(request.Context(), cookie.Value)
		if err != nil {
			s.clearAuthCookies(response)
			s.writeError(response, request, err)
			return
		}
		if requireCSRF {
			csrfCookie, cookieErr := request.Cookie(csrfCookieName)
			headerToken := request.Header.Get(csrfHeader)
			if cookieErr != nil || csrfCookie.Value == "" || headerToken == "" || headerToken != csrfCookie.Value || !s.auth.ValidateCSRF(session, headerToken) {
				s.writeError(response, request, domain.NewError(domain.ErrorAuthorisation, "the CSRF token is missing or invalid"))
				return
			}
		}
		ctx := context.WithValue(request.Context(), sessionKey, session)
		ctx = context.WithValue(ctx, userKey, user)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func (s *Server) administrator(requireCSRF bool, next http.Handler) http.Handler {
	return s.authenticated(requireCSRF, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if authenticatedUser(request.Context()).Role != domain.RoleAdministrator {
			s.writeError(response, request, domain.NewError(domain.ErrorAuthorisation, "administrator access is required"))
			return
		}
		next.ServeHTTP(response, request)
	}))
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := strings.TrimSpace(request.Header.Get(requestIDHeader))
		if !domain.ValidID(id) {
			generated, err := domain.NewID()
			if err != nil {
				http.Error(response, "request identifier unavailable", http.StatusInternalServerError)
				return
			}
			id = generated
		}
		response.Header().Set(requestIDHeader, id)
		next.ServeHTTP(response, request.WithContext(context.WithValue(request.Context(), requestIDKey, id)))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(request.URL.Path, "/api/") {
			response.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(response, request)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *statusWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

func (writer *statusWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writer := &statusWriter{ResponseWriter: response, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(writer, request)
		s.logger.Info("http request completed",
			"request_id", requestID(request.Context()), "method", request.Method,
			"path", request.URL.Path, "status", writer.status,
			"duration_ms", time.Since(started).Milliseconds())
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("http handler panic", "request_id", requestID(request.Context()), "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
				s.writeError(response, request, errors.New("handler panic"))
			}
		}()
		next.ServeHTTP(response, request)
	})
}

func (s *Server) setAuthCookies(response http.ResponseWriter, result auth.SessionResult) {
	maxAge := int(time.Until(result.Session.ExpiresAt).Seconds())
	http.SetCookie(response, &http.Cookie{
		Name: sessionCookieName, Value: result.Token, Path: "/", MaxAge: maxAge,
		Expires: result.Session.ExpiresAt, HttpOnly: true, Secure: s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(response, &http.Cookie{
		Name: csrfCookieName, Value: result.CSRFToken, Path: "/", MaxAge: maxAge,
		Expires: result.Session.ExpiresAt, HttpOnly: false, Secure: s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearAuthCookies(response http.ResponseWriter) {
	expired := time.Unix(1, 0)
	for _, cookie := range []*http.Cookie{
		{Name: sessionCookieName, HttpOnly: true},
		{Name: csrfCookieName, HttpOnly: false},
	} {
		cookie.Value = ""
		cookie.Path = "/"
		cookie.MaxAge = -1
		cookie.Expires = expired
		cookie.Secure = s.secureCookies
		cookie.SameSite = http.SameSiteStrictMode
		http.SetCookie(response, cookie)
	}
}

func remoteIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func (s *Server) handleFrontend(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		s.writeError(response, request, domain.NewError(domain.ErrorNotFound, "route was not found"))
		return
	}
	if strings.HasPrefix(request.URL.Path, "/api/") {
		s.writeError(response, request, domain.NewError(domain.ErrorNotFound, "API route was not found"))
		return
	}
	relative := strings.TrimPrefix(filepath.Clean(request.URL.Path), string(filepath.Separator))
	if relative == "." || relative == "" {
		relative = "index.html"
	}
	filePath := filepath.Join(s.webDist, relative)
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(response, request, filePath)
		return
	}
	indexPath := filepath.Join(s.webDist, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte("Atlas DNS Controller frontend has not been built.\n"))
		return
	}
	http.ServeFile(response, request, indexPath)
}
