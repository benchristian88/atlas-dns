package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/operations"
)

func TestConfigurationInventoryResponseOmitsMissingDraft(t *testing.T) {
	body, err := json.Marshal(configurationInventoryResponse{
		SchemaVersion: configuration.SchemaVersion,
		Snapshots:     nil,
		Capabilities:  nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if _, exists := response["draft"]; exists {
		t.Fatalf("missing draft must be omitted, response was %s", body)
	}
}

func TestPresentationEndpointsRequireAuthentication(t *testing.T) {
	for _, path := range []string{
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/blocked-services/catalogue",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/blocklists/presentation",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/allowlists/presentation",
		"/api/v1/nodes/22222222-2222-4222-8222-222222222222/dhcp/interfaces",
		"/api/v1/operational-commands/77777777-7777-4777-8777-777777777777",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/statistics",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/query-events",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/query-events/33333333-3333-4333-8333-333333333333",
	} {
		t.Run(path, func(t *testing.T) {
			server := &Server{mux: http.NewServeMux()}
			server.routes()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			server.mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "AUTHENTICATION_REQUIRED" {
				t.Fatalf("unexpected safe error body: %s", response.Body.String())
			}
		})
	}
}

func TestDHCPOperationEndpointsRequireAuthentication(t *testing.T) {
	for path, body := range map[string]string{
		"/api/v1/nodes/22222222-2222-4222-8222-222222222222/dhcp/active-check":        `{"interfaceName":"eth0"}`,
		"/api/v1/nodes/22222222-2222-4222-8222-222222222222/dhcp/reset-leases":        `{"confirmation":"RESET_LEASES"}`,
		"/api/v1/nodes/22222222-2222-4222-8222-222222222222/dhcp/reset-configuration": `{"confirmation":"RESET_DHCP_CONFIGURATION"}`,
	} {
		t.Run(path, func(t *testing.T) {
			server := &Server{mux: http.NewServeMux()}
			server.routes()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			response := httptest.NewRecorder()
			server.mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"AUTHENTICATION_REQUIRED"`) {
				t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDNSOperationEndpointsRequireAuthentication(t *testing.T) {
	for _, path := range []string{
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/test-upstream-dns",
		"/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/clear-dns-cache",
	} {
		server := &Server{mux: http.NewServeMux()}
		server.routes()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		response := httptest.NewRecorder()
		server.mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"AUTHENTICATION_REQUIRED"`) {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

type catalogueReaderFake struct {
	result inventory.BlockedServicesCatalogue
	err    error
}

type blocklistReaderFake struct {
	result inventory.BlocklistPresentation
	err    error
}

type allowlistReaderFake struct {
	result inventory.AllowlistPresentation
	err    error
}

type dhcpInterfacesReaderFake struct {
	result inventory.DHCPInterfaces
	err    error
}

func (f dhcpInterfacesReaderFake) DHCPInterfaces(context.Context, string) (inventory.DHCPInterfaces, error) {
	return f.result, f.err
}

type dhcpCheckerFake struct {
	result inventory.DHCPActiveCheckResult
	err    error
}

type dhcpOperationsFake struct {
	result       inventory.DHCPOperation
	items        []inventory.DHCPOperation
	err          error
	calls        int
	nodeID       string
	command      inventory.DHCPOperationCommand
	confirmation string
	idempotency  string
}

type dnsOperationsFake struct {
	result operations.Operation
	calls  int
	target operations.Target
}

func (f *dnsOperationsFake) StartUpstreamTest(_ context.Context, _ domain.Actor, _ string, target operations.Target, _ operations.UpstreamInput, _ string) (operations.Operation, error) {
	f.calls++
	f.target = target
	return f.result, nil
}
func (f *dnsOperationsFake) StartHostFilterTest(_ context.Context, _ domain.Actor, _ string, target operations.Target, _ operations.HostFilterInput, _ string) (operations.Operation, error) {
	f.calls++
	f.target = target
	return f.result, nil
}
func (f *dnsOperationsFake) StartCacheClear(_ context.Context, _ domain.Actor, _ string, target operations.Target, _ string, _ string) (operations.Operation, error) {
	f.calls++
	f.target = target
	return f.result, nil
}
func (f *dnsOperationsFake) StartQueryLogClear(_ context.Context, _ domain.Actor, _ string, target operations.Target, _ string, _ string) (operations.Operation, error) {
	f.calls++
	f.target = target
	return f.result, nil
}
func (f *dnsOperationsFake) StartStatisticsReset(_ context.Context, _ domain.Actor, _ string, target operations.Target, _ string, _ string) (operations.Operation, error) {
	f.calls++
	f.target = target
	return f.result, nil
}
func (f *dnsOperationsFake) Operation(context.Context, string) (operations.Operation, error) {
	return f.result, nil
}
func (f *dnsOperationsFake) List(context.Context, string, operations.Command, int) ([]operations.Operation, error) {
	return []operations.Operation{f.result}, nil
}

func (f *dhcpOperationsFake) RunDHCPOperation(_ context.Context, _ domain.Actor, nodeID string, command inventory.DHCPOperationCommand, confirmation, idempotency string) (inventory.DHCPOperation, error) {
	f.calls++
	f.nodeID, f.command, f.confirmation, f.idempotency = nodeID, command, confirmation, idempotency
	return f.result, f.err
}

func (f *dhcpOperationsFake) ListDHCPOperations(context.Context, string, int) ([]inventory.DHCPOperation, error) {
	return f.items, f.err
}

func (f dhcpCheckerFake) FindActiveDHCP(context.Context, domain.Actor, string, string) (inventory.DHCPActiveCheckResult, error) {
	return f.result, f.err
}

func (f blocklistReaderFake) BlocklistPresentation(context.Context, string) (inventory.BlocklistPresentation, error) {
	return f.result, f.err
}

func (f allowlistReaderFake) AllowlistPresentation(context.Context, string) (inventory.AllowlistPresentation, error) {
	return f.result, f.err
}

func TestBlocklistPresentationEndpointReturnsOnlySanitisedMetadata(t *testing.T) {
	server := &Server{
		blocklists: blocklistReaderFake{result: inventory.BlocklistPresentation{
			Nodes: []inventory.BlocklistNodePresentation{{NodeID: "node-a", NodeName: "Primary", Status: "available", Lists: []inventory.FilterListMetadata{{ID: 7, URL: "https://filters.test/list.txt", Name: "Example", Enabled: true, RulesCount: 321, Portable: true}}}},
		}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/11111111-1111-4111-8111-111111111111/blocklists/presentation", nil)
	request.SetPathValue("clusterId", "11111111-1111-4111-8111-111111111111")
	response := httptest.NewRecorder()
	server.handleBlocklistPresentation(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ruleCount":321`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"credentials", "baseUrl", "canonicalHash", "document"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestAllowlistPresentationEndpointReturnsOnlySanitisedMetadata(t *testing.T) {
	server := &Server{
		allowlists: allowlistReaderFake{result: inventory.AllowlistPresentation{
			Nodes: []inventory.BlocklistNodePresentation{{NodeID: "node-a", NodeName: "Primary", Status: "available", Lists: []inventory.FilterListMetadata{{ID: 8, URL: "https://allow.test/list.txt", Name: "Allow", Enabled: true, RulesCount: 12, Portable: true}}}},
		}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/11111111-1111-4111-8111-111111111111/allowlists/presentation", nil)
	request.SetPathValue("clusterId", "11111111-1111-4111-8111-111111111111")
	response := httptest.NewRecorder()
	server.handleAllowlistPresentation(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ruleCount":12`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"credentials", "baseUrl", "canonicalHash", "document"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response.Body.String())
		}
	}
}

func (f catalogueReaderFake) BlockedServicesCatalogue(context.Context, string) (inventory.BlockedServicesCatalogue, error) {
	return f.result, f.err
}

func TestBlockedServicesCatalogueEndpointReturnsSanitisedControllerDTO(t *testing.T) {
	server := &Server{
		catalogue: catalogueReaderFake{result: inventory.BlockedServicesCatalogue{
			Services: []inventory.MergedBlockedService{{ID: "youtube", Name: "YouTube", SupportedNodeIDs: []string{"node-a"}, UnsupportedNodeIDs: []string{}}},
			Groups:   []inventory.BlockedServiceGroup{},
			Nodes:    []inventory.BlockedServicesCatalogueNode{{NodeID: "node-a", NodeName: "Primary", Status: "available", ServiceCount: 1}},
		}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/11111111-1111-4111-8111-111111111111/blocked-services/catalogue", nil)
	request.SetPathValue("clusterId", "11111111-1111-4111-8111-111111111111")
	response := httptest.NewRecorder()
	server.handleBlockedServicesCatalogue(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"YouTube"`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"icon_svg", "rules", "baseUrl", "credentials"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestBlockedServicesCatalogueEndpointMapsInternalErrorsSafely(t *testing.T) {
	server := &Server{
		catalogue: catalogueReaderFake{err: errors.New("secret node response from http://private-node.test")},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/11111111-1111-4111-8111-111111111111/blocked-services/catalogue", nil)
	request.SetPathValue("clusterId", "11111111-1111-4111-8111-111111111111")
	response := httptest.NewRecorder()
	server.handleBlockedServicesCatalogue(response, request)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "private-node") {
		t.Fatalf("unsafe error response: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("stable error code missing: %s", response.Body.String())
	}
}

func TestDHCPInterfacesEndpointReturnsSanitisedNodeMetadata(t *testing.T) {
	server := &Server{dhcpInterfaces: dhcpInterfacesReaderFake{result: inventory.DHCPInterfaces{
		NodeID: "node-a", NodeName: "Primary", Interfaces: []inventory.DHCPInterface{{Name: "eth0", IPv4Addresses: []string{"192.0.2.2"}, IPv6Addresses: []string{}, Flags: []string{"up"}, Available: true}},
	}}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-a/dhcp/interfaces", nil)
	request.SetPathValue("nodeId", "node-a")
	response := httptest.NewRecorder()
	server.handleDHCPInterfaces(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":true`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"baseUrl", "credentials", "customCaPem"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestDHCPActiveCheckEndpointReturnsControllerResult(t *testing.T) {
	server := &Server{dhcpChecker: dhcpCheckerFake{result: inventory.DHCPActiveCheckResult{
		NodeID: "node-a", NodeName: "Primary", InterfaceName: "eth0", Status: "none",
		IPv4: inventory.DHCPCheckValue{Status: "no"}, IPv6: inventory.DHCPCheckValue{Status: "no"},
	}}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-a/dhcp/active-check", strings.NewReader(`{"interfaceName":"eth0"}`))
	request.SetPathValue("nodeId", "node-a")
	response := httptest.NewRecorder()
	server.handleDHCPActiveCheck(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"none"`) || !strings.Contains(response.Body.String(), `"interfaceName":"eth0"`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDHCPResetHandlersKeepCommandsSeparateAndReturnDurableResult(t *testing.T) {
	for _, test := range []struct {
		name, path, confirmation string
		command                  inventory.DHCPOperationCommand
	}{
		{name: "leases", path: "/api/v1/nodes/node-a/dhcp/reset-leases", confirmation: "RESET_LEASES", command: inventory.DHCPOperationResetLeases},
		{name: "configuration", path: "/api/v1/nodes/node-a/dhcp/reset-configuration", confirmation: "RESET_DHCP_CONFIGURATION", command: inventory.DHCPOperationResetConfiguration},
	} {
		t.Run(test.name, func(t *testing.T) {
			operation := inventory.DHCPOperation{ID: "operation-a", Status: "succeeded", RequestID: "request-a", AuditReference: "audit-a", NodeResults: []inventory.DHCPOperationNodeResult{{NodeID: "node-a", Status: "succeeded"}}}
			service := &dhcpOperationsFake{result: operation}
			server := &Server{dhcpOperations: service, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"confirmation":"`+test.confirmation+`"}`))
			request.SetPathValue("nodeId", "node-a")
			request.Header.Set(idempotencyHeader, "55555555-5555-4555-8555-555555555555")
			response := httptest.NewRecorder()
			server.handleDHCPOperation(response, request, test.command)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"auditReference":"audit-a"`) || !strings.Contains(response.Body.String(), `"requestId":"request-a"`) {
				t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
			}
			if service.calls != 1 || service.command != test.command || service.confirmation != test.confirmation || service.nodeID != "node-a" {
				t.Fatalf("service call = %#v", service)
			}
		})
	}
}

type apiAuthRepositoryFake struct {
	session domain.Session
	user    domain.User
}

func (*apiAuthRepositoryFake) HasUsers(context.Context) (bool, error) { return true, nil }
func (*apiAuthRepositoryFake) CreateInitialUser(context.Context, domain.User, domain.Session, domain.AuditEvent, domain.AuditEvent) error {
	return nil
}
func (*apiAuthRepositoryFake) UserByEmail(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.NewError(domain.ErrorNotFound, "user not found")
}
func (*apiAuthRepositoryFake) CreateLoginSession(context.Context, domain.Session, domain.AuditEvent) error {
	return nil
}
func (f *apiAuthRepositoryFake) AuthenticatedSession(context.Context, []byte, time.Time) (domain.Session, domain.User, error) {
	return f.session, f.user, nil
}
func (*apiAuthRepositoryFake) TouchSession(context.Context, string, time.Time) error { return nil }
func (*apiAuthRepositoryFake) RevokeSession(context.Context, string, time.Time, domain.AuditEvent) error {
	return nil
}
func (*apiAuthRepositoryFake) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func TestDHCPResetEndpointRequiresCSRFAndAcceptsMatchingToken(t *testing.T) {
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "csrf-token"
	repository := &apiAuthRepositoryFake{
		session: domain.Session{ID: "session-a", CSRFHash: tokens.HashCSRFToken(csrf)},
		user:    domain.User{ID: "33333333-3333-4333-8333-333333333333", Enabled: true},
	}
	authService, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	operations := &dhcpOperationsFake{result: inventory.DHCPOperation{ID: "operation-a", Status: "succeeded", NodeResults: []inventory.DHCPOperationNodeResult{{NodeID: "22222222-2222-4222-8222-222222222222", Status: "succeeded"}}}}
	server := &Server{auth: authService, dhcpOperations: operations, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()
	path := "/api/v1/nodes/22222222-2222-4222-8222-222222222222/dhcp/reset-leases"

	withoutCSRF := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"confirmation":"RESET_LEASES"}`))
	withoutCSRF.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	withoutResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(withoutResponse, withoutCSRF)
	if withoutResponse.Code != http.StatusForbidden || operations.calls != 0 || !strings.Contains(withoutResponse.Body.String(), `"code":"AUTHORISATION_DENIED"`) {
		t.Fatalf("missing CSRF response: status=%d calls=%d body=%s", withoutResponse.Code, operations.calls, withoutResponse.Body.String())
	}

	withCSRF := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"confirmation":"RESET_LEASES"}`))
	withCSRF.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	withCSRF.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	withCSRF.Header.Set(csrfHeader, csrf)
	withCSRF.Header.Set(idempotencyHeader, "55555555-5555-4555-8555-555555555555")
	withResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(withResponse, withCSRF)
	if withResponse.Code != http.StatusOK || operations.calls != 1 {
		t.Fatalf("matching CSRF response: status=%d calls=%d body=%s", withResponse.Code, operations.calls, withResponse.Body.String())
	}
}

func TestDNSOperationRequiresCSRFAndReturnsQueuedResource(t *testing.T) {
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "csrf-token"
	repository := &apiAuthRepositoryFake{
		session: domain.Session{ID: "session-a", CSRFHash: tokens.HashCSRFToken(csrf)},
		user:    domain.User{ID: "33333333-3333-4333-8333-333333333333", Enabled: true},
	}
	authService, err := auth.NewService(repository, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service := &dnsOperationsFake{result: operations.Operation{ID: "operation-a", Status: "queued", Command: operations.TestUpstreamDNS, NodeResults: []operations.NodeResult{{NodeID: "node-a", NodeName: "Primary", Status: "pending"}}}}
	server := &Server{auth: authService, dnsOperations: service, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), mux: http.NewServeMux()}
	server.routes()
	path := "/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/test-upstream-dns"
	body := `{"target":{"scope":"node","nodeId":"22222222-2222-4222-8222-222222222222"},"input":{"draftVersion":4,"upstreamDns":["1.1.1.1"],"bootstrapDns":[],"fallbackDns":[],"privateReverseDns":[],"upstreamMode":"load_balance","usePrivateReverseResolvers":false}}`

	without := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	without.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	withoutResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(withoutResponse, without)
	if withoutResponse.Code != http.StatusForbidden || service.calls != 0 {
		t.Fatalf("missing CSRF status=%d calls=%d", withoutResponse.Code, service.calls)
	}

	with := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	with.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	with.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	with.Header.Set(csrfHeader, csrf)
	with.Header.Set(idempotencyHeader, "55555555-5555-4555-8555-555555555555")
	response := httptest.NewRecorder()
	server.mux.ServeHTTP(response, with)
	if response.Code != http.StatusAccepted || service.calls != 1 || service.target.NodeID == "" || !strings.Contains(response.Body.String(), `"status":"queued"`) {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
	}

	hostPath := "/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/test-host-filtering"
	hostBody := `{"target":{"scope":"all_compatible_enabled_nodes"},"input":{"hostname":"ads.example","client":"192.0.2.10","queryType":"AAAA"}}`
	unauthenticated := httptest.NewRequest(http.MethodPost, hostPath, strings.NewReader(hostBody))
	unauthenticatedResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized || service.calls != 1 {
		t.Fatalf("unauthenticated status=%d calls=%d", unauthenticatedResponse.Code, service.calls)
	}
	hostRequest := httptest.NewRequest(http.MethodPost, hostPath, strings.NewReader(hostBody))
	hostRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	hostRequest.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	hostRequest.Header.Set(csrfHeader, csrf)
	hostRequest.Header.Set(idempotencyHeader, "66666666-6666-4666-8666-666666666666")
	hostResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(hostResponse, hostRequest)
	if hostResponse.Code != http.StatusAccepted || service.calls != 2 || service.target.Scope != "all_compatible_enabled_nodes" {
		t.Fatalf("host status=%d calls=%d body=%s", hostResponse.Code, service.calls, hostResponse.Body.String())
	}

	queryPath := "/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/clear-query-log"
	queryBody := `{"target":{"scope":"node","nodeId":"22222222-2222-4222-8222-222222222222"},"confirmation":"CLEAR_QUERY_LOG"}`
	queryWithoutCSRF := httptest.NewRequest(http.MethodPost, queryPath, strings.NewReader(queryBody))
	queryWithoutCSRF.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	queryWithoutResponse := httptest.NewRecorder()
	server.mux.ServeHTTP(queryWithoutResponse, queryWithoutCSRF)
	if queryWithoutResponse.Code != http.StatusForbidden || service.calls != 2 {
		t.Fatalf("query missing CSRF status=%d calls=%d", queryWithoutResponse.Code, service.calls)
	}
	for index, item := range []struct {
		path string
		body string
	}{
		{queryPath, queryBody},
		{"/api/v1/clusters/11111111-1111-4111-8111-111111111111/operational-commands/reset-statistics", `{"target":{"scope":"all_compatible_enabled_nodes"},"confirmation":"RESET_STATISTICS"}`},
	} {
		request := httptest.NewRequest(http.MethodPost, item.path, strings.NewReader(item.body))
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
		request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		request.Header.Set(csrfHeader, csrf)
		request.Header.Set(idempotencyHeader, fmt.Sprintf("77777777-7777-4777-8777-77777777777%d", index))
		result := httptest.NewRecorder()
		server.mux.ServeHTTP(result, request)
		if result.Code != http.StatusAccepted {
			t.Fatalf("policy operation %s status=%d body=%s", item.path, result.Code, result.Body.String())
		}
	}
	if service.calls != 4 {
		t.Fatalf("policy operation calls=%d", service.calls)
	}
}

func TestNoFleetWideDHCPResetRouteExists(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.routes()
	for _, path := range []string{"/api/v1/dhcp/reset-leases", "/api/v1/clusters/11111111-1111-4111-8111-111111111111/dhcp/reset-configuration"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"confirmation":"RESET_LEASES"}`))
		response := httptest.NewRecorder()
		server.mux.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("fleet reset path %s returned %d", path, response.Code)
		}
	}
}
