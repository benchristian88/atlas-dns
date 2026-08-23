package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/controlplane"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

type adGuardState struct {
	mu              sync.Mutex
	upstreamDNS     []string
	userRules       []string
	blockedServices []string
	catalogue       []map[string]any
	clients         []map[string]any
	rewrites        []map[string]any
}

func (s *adGuardState) handler(response http.ResponseWriter, request *http.Request) {
	username, password, ok := request.BasicAuth()
	if !ok || username != "admin" || password != "secret" {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	response.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/control/status":
		_, _ = io.WriteString(response, validAdGuardStatusResponse("v0.107.65"))
	case "/control/dns_info":
		_ = json.NewEncoder(response).Encode(map[string]any{"upstream_dns": s.upstreamDNS, "bootstrap_dns": []string{}, "fallback_dns": []string{}, "local_ptr_upstreams": []string{}, "protection_enabled": true, "protection_disabled_until": nil, "cache_enabled": true, "cache_size": 4_194_304, "upstream_timeout": 10})
	case "/control/filtering/status":
		_ = json.NewEncoder(response).Encode(map[string]any{"enabled": true, "interval": 24, "filters": []any{}, "whitelist_filters": []any{}, "user_rules": s.userRules})
	case "/control/clients":
		_ = json.NewEncoder(response).Encode(map[string]any{"clients": s.clients})
	case "/control/rewrite/list":
		_ = json.NewEncoder(response).Encode(s.rewrites)
	case "/control/rewrite/add":
		var rewrite map[string]any
		_ = json.NewDecoder(request.Body).Decode(&rewrite)
		s.rewrites = append(s.rewrites, rewrite)
	case "/control/rewrite/update":
		var body struct {
			Target map[string]any `json:"target"`
			Update map[string]any `json:"update"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		for index, rewrite := range s.rewrites {
			if rewrite["domain"] == body.Target["domain"] && rewrite["answer"] == body.Target["answer"] {
				s.rewrites[index] = body.Update
				break
			}
		}
	case "/control/rewrite/delete":
		var target map[string]any
		_ = json.NewDecoder(request.Body).Decode(&target)
		for index, rewrite := range s.rewrites {
			if rewrite["domain"] == target["domain"] && rewrite["answer"] == target["answer"] {
				s.rewrites = append(s.rewrites[:index], s.rewrites[index+1:]...)
				break
			}
		}
	case "/control/blocked_services/all":
		_ = json.NewEncoder(response).Encode(map[string]any{"blocked_services": s.catalogue})
	case "/control/blocked_services/get":
		_ = json.NewEncoder(response).Encode(map[string]any{"ids": s.blockedServices, "schedule": map[string]any{"time_zone": "Local"}})
	case "/control/safebrowsing/status", "/control/parental/status":
		_, _ = io.WriteString(response, `{"enabled":false}`)
	case "/control/safesearch/status":
		_, _ = io.WriteString(response, `{"enabled":false,"bing":true,"duckduckgo":true,"ecosia":true,"google":true,"pixabay":true,"yandex":true,"youtube":true}`)
	case "/control/querylog/config", "/control/stats/config":
		_, _ = io.WriteString(response, `{"enabled":false,"interval":0,"ignored":[]}`)
	case "/control/tls/status":
		_, _ = io.WriteString(response, `{"enabled":false,"serve_plain_dns":true}`)
	case "/control/dhcp/status":
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(response, `{"message":"DHCP unavailable"}`)
	case "/control/dns_config":
		var body struct {
			UpstreamDNS []string `json:"upstream_dns"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		s.upstreamDNS = append([]string(nil), body.UpstreamDNS...)
	case "/control/filtering/set_rules":
		var body struct {
			Rules []string `json:"rules"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		s.userRules = append([]string(nil), body.Rules...)
	case "/control/blocked_services/update":
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		s.blockedServices = append([]string(nil), body.IDs...)
	case "/control/clients/add":
		var client map[string]any
		_ = json.NewDecoder(request.Body).Decode(&client)
		s.clients = append(s.clients, client)
	case "/control/clients/update":
		var body struct {
			Name string         `json:"name"`
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		for index, client := range s.clients {
			if client["name"] == body.Name {
				s.clients[index] = body.Data
				break
			}
		}
	case "/control/clients/delete":
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		for index, client := range s.clients {
			if client["name"] == body.Name {
				s.clients = append(s.clients[:index], s.clients[index+1:]...)
				break
			}
		}
	default:
		if request.Method != http.MethodPost && request.Method != http.MethodPut {
			http.NotFound(response, request)
		}
	}
}

func (s *adGuardState) blockedServiceIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.blockedServices...)
}

func (s *adGuardState) clientValues() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]map[string]any, len(s.clients))
	for index, client := range s.clients {
		values[index] = make(map[string]any, len(client))
		for key, value := range client {
			values[index][key] = value
		}
	}
	return values
}

func (s *adGuardState) setClientFiltering(name string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.clients {
		if client["name"] == name {
			client["filtering_enabled"] = enabled
			return
		}
	}
}

func (s *adGuardState) rewriteValues() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]map[string]any, len(s.rewrites))
	for index, rewrite := range s.rewrites {
		values[index] = make(map[string]any, len(rewrite))
		for key, value := range rewrite {
			values[index][key] = value
		}
	}
	return values
}

func (s *adGuardState) setRewriteAnswer(domain, answer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rewrite := range s.rewrites {
		if rewrite["domain"] == domain {
			rewrite["answer"] = answer
			return
		}
	}
}

func TestRelease03AuthoritativeConfigurationWorkflow(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	credentialCipher, err := auth.NewCredentialCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(store, tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	requestID := mustID(t)
	setup, err := authService.Setup(ctx, "release03@example.test", "Release 0.3", "correct horse battery staple", requestID, "127.0.0.1", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	actor := domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}
	probe := adguard.NewProbe(2 * time.Second)
	management := domain.NewManagementService(store, credentialCipher, probe)
	cluster, err := management.CreateCluster(ctx, actor, domain.CreateClusterInput{Name: "Release 0.3", ReconciliationPolicy: domain.ReconciliationEnforce})
	if err != nil {
		t.Fatal(err)
	}
	states := []*adGuardState{
		{upstreamDNS: []string{"9.9.9.9"}, catalogue: []map[string]any{{"id": "youtube", "name": "YouTube"}, {"id": "chatgpt", "name": "ChatGPT"}}},
		{upstreamDNS: []string{"9.9.9.9"}, catalogue: []map[string]any{{"id": "youtube", "name": "YouTube"}}},
	}
	servers := make([]*httptest.Server, 0, len(states))
	for _, state := range states {
		server := httptest.NewServer(http.HandlerFunc(state.handler))
		servers = append(servers, server)
		t.Cleanup(server.Close)
	}
	nodes := make([]domain.Node, 0, 2)
	for index, server := range servers {
		node, err := management.CreateNode(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, domain.CreateNodeInput{ClusterID: cluster.ID, Name: "Node " + string(rune('A'+index)), BaseURL: server.URL, CertificatePolicy: domain.CertificateInsecureHTTP, Username: "admin", Password: "secret", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, node)
	}
	adapter := adguard.NewConfigurationReader(probe)
	inventoryService := inventory.NewService(store, credentialCipher, adapter)
	var draft inventory.Draft
	for index, node := range nodes {
		snapshot, err := inventoryService.Observe(ctx, node.ID)
		if err != nil {
			t.Fatal(err)
		}
		draft, err = inventoryService.Import(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, snapshot.ID, index, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	service := controlplane.NewService(store, inventoryService)
	// Exercise the operator's initial workflow exactly: import every node, edit
	// shared desired state, and save before any revision is published or active.
	draft.Document.Shared.DNS.UpstreamDNS = []string{"9.9.9.9", "149.112.112.112"}
	draft.Document.Shared.Services.BlockedServiceIDs = []string{"youtube"}
	draft.Document.Shared.Rewrites = []configuration.Rewrite{
		{Domain: "router.test", Answer: "192.0.2.1", Enabled: true},
		{Domain: "*.service.test", Answer: "target.test", Enabled: true},
	}
	draft.Document.Shared.Clients = []configuration.PersistentClient{{
		Name:                     "Printer",
		IDs:                      []string{"192.0.2.10", "printer-client-id"},
		UseGlobalSettings:        false,
		FilteringEnabled:         true,
		ParentalEnabled:          false,
		SafeBrowsingEnabled:      true,
		SafeSearch:               configuration.SafeSearch{Enabled: true, Bing: true, DuckDuckGo: false, Ecosia: true, Google: true, Pixabay: true, Yandex: true, YouTube: true},
		UseGlobalBlockedServices: false,
		BlockedServices:          []string{"youtube"},
		BlockedServicesSchedule:  configuration.Schedule{TimeZone: "Local", Days: map[string]configuration.DayRange{}},
		Upstreams:                []string{"tls://dns.example", "1.1.1.1"},
		UpstreamsCacheEnabled:    true,
		UpstreamsCacheSize:       2_097_152,
		Tags:                     []string{"device_printer", "legacy_tag"},
		IgnoreQueryLog:           true,
		IgnoreStatistics:         false,
	}}
	draft, issues, err := service.UpdateDraft(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, draft.Version, draft.Document)
	if err != nil || len(issues) != 0 {
		t.Fatalf("update imported draft issues=%v error=%v", issues, err)
	}
	revisionOne, err := service.Publish(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, "Initial authoritative DNS policy", draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.StartDeployment(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, revisionOne.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := controlplane.NewExecutor(store, credentialCipher, adapter, inventoryService)
	if worked, err := executor.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("initial deployment worked=%v error=%v", worked, err)
	}
	assertDeploymentSucceeded(t, service, deployment.ID, 2)
	for index, state := range states {
		ids := state.blockedServiceIDs()
		if len(ids) != 1 || ids[0] != "youtube" {
			t.Fatalf("node %d blocked services = %#v", index, ids)
		}
		clients := state.clientValues()
		if len(clients) != 1 || clients[0]["name"] != "Printer" || clients[0]["filtering_enabled"] != true || clients[0]["ignore_querylog"] != true || clients[0]["upstreams_cache_size"] != float64(2_097_152) {
			t.Fatalf("node %d persistent clients = %#v", index, clients)
		}
		upstreams, ok := clients[0]["upstreams"].([]any)
		if !ok || len(upstreams) != 2 || upstreams[0] != "tls://dns.example" || upstreams[1] != "1.1.1.1" {
			t.Fatalf("node %d client upstream order = %#v", index, clients[0]["upstreams"])
		}
		rewrites := state.rewriteValues()
		if len(rewrites) != 2 || !hasRewrite(rewrites, "router.test", "192.0.2.1") || !hasRewrite(rewrites, "*.service.test", "target.test") {
			t.Fatalf("node %d DNS rewrites = %#v", index, rewrites)
		}
	}

	// A union catalogue entry that one enabled node does not support remains in
	// the draft but blocks immutable publication with node-attributed preflight.
	draft, err = store.DraftByCluster(ctx, cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft.Document.Shared.Services.BlockedServiceIDs = []string{"youtube", "chatgpt"}
	draft, issues, err = service.UpdateDraft(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, draft.Version, draft.Document)
	if err != nil || len(issues) != 0 {
		t.Fatalf("save incompatible blocked service issues=%v error=%v", issues, err)
	}
	if _, err := service.Publish(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, "Unsupported service", draft.Version); err == nil {
		t.Fatal("publication accepted a service unsupported by one node")
	}
	draft.Document.Shared.Services.BlockedServiceIDs = []string{"youtube"}
	draft, issues, err = service.UpdateDraft(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, draft.Version, draft.Document)
	if err != nil || len(issues) != 0 {
		t.Fatalf("restore compatible blocked service issues=%v error=%v", issues, err)
	}

	states[0].setClientFiltering("Printer", false)
	states[0].setRewriteAnswer("router.test", "192.0.2.99")
	reconciler := controlplane.NewReconciler(store, service, inventoryService, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	drift, err := service.ListDrift(ctx, cluster.ID)
	if err != nil || len(drift) != 1 || drift[0].Status != "open" || drift[0].RelatedDeploymentID == nil {
		t.Fatalf("automatic drift record = %#v, error=%v", drift, err)
	}
	if worked, err := executor.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("reconciliation deployment worked=%v error=%v", worked, err)
	}
	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	drift, err = service.ListDrift(ctx, cluster.ID)
	if err != nil || drift[0].Status != "resolved" {
		t.Fatalf("resolved drift = %#v, error=%v", drift, err)
	}
	if clients := states[0].clientValues(); len(clients) != 1 || clients[0]["filtering_enabled"] != true {
		t.Fatalf("client drift did not converge: %#v", clients)
	}
	if rewrites := states[0].rewriteValues(); !hasRewrite(rewrites, "router.test", "192.0.2.1") || hasRewrite(rewrites, "router.test", "192.0.2.99") {
		t.Fatalf("rewrite drift did not converge: %#v", rewrites)
	}

	draft, err = store.DraftByCluster(ctx, cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft.Document.Shared.DNS.UpstreamDNS = []string{"1.1.1.1"}
	draft, issues, err = service.UpdateDraft(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, draft.Version, draft.Document)
	if err != nil || len(issues) != 0 {
		t.Fatalf("update second draft issues=%v error=%v", issues, err)
	}
	revisionTwo, err := service.Publish(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, "Use Cloudflare upstream", draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.StartDeployment(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, revisionTwo.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	assertDeploymentSucceeded(t, service, second.ID, 2)
	rollback, err := service.Rollback(ctx, domain.Actor{UserID: setup.User.ID, RequestID: mustID(t)}, cluster.ID, revisionOne.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	assertDeploymentSucceeded(t, service, rollback.ID, 2)
	active, err := store.ClusterByID(ctx, cluster.ID)
	if err != nil || active.ActiveRevisionID == nil || *active.ActiveRevisionID != revisionOne.ID {
		t.Fatalf("active revision = %#v, error=%v", active.ActiveRevisionID, err)
	}
}

func hasRewrite(rewrites []map[string]any, domain, answer string) bool {
	for _, rewrite := range rewrites {
		if rewrite["domain"] == domain && rewrite["answer"] == answer {
			return true
		}
	}
	return false
}

func assertDeploymentSucceeded(t *testing.T, service *controlplane.Service, id string, nodeCount int) {
	t.Helper()
	deployment, err := service.Deployment(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Status != "succeeded" || len(deployment.Nodes) != nodeCount {
		t.Fatalf("deployment = %#v", deployment)
	}
	for _, node := range deployment.Nodes {
		if node.Status != "succeeded" || node.VerificationSnapshotID == nil {
			t.Fatalf("deployment node = %#v", node)
		}
	}
}

func mustID(t *testing.T) string {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
