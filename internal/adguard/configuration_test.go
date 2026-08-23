package adguard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/operations"
)

func TestVersionFixturesSuppressVolatileFields(t *testing.T) {
	documents := make([]configuration.Document, 0, 2)
	for _, version := range []string{"v0.107.52", "v0.107.61"} {
		var status statusResponse
		readFixture(t, filepath.Join("testdata", version, "status.json"), &status)
		var dns dnsInfoResponse
		readFixture(t, filepath.Join("testdata", version, "dns_info.json"), &dns)
		var filtering filterStatusResponse
		readFixture(t, filepath.Join("testdata", version, "filtering_status.json"), &filtering)
		documents = append(documents, configurationDocument(version, status, dns, filtering))
	}
	if differences := configuration.Diff(documents[0], documents[1]); len(differences) != 0 {
		t.Fatalf("equivalent fixtures differ: %#v", differences)
	}
}

func TestReadBlocklistsPreservesVolatileMetadataOutsideConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/filtering/status" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"enabled":true,"filters":[{"id":7,"enabled":true,"url":"https://filters.test/list.txt","name":"Primary list","rules_count":321,"last_updated":"2026-08-01T01:02:03Z"},{"id":8,"enabled":false,"url":"/opt/adguard/local.txt","name":"Local list","rules_count":4}]}`))
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	lists, err := adapter.ReadBlocklists(context.Background(), probeRequest(server.URL), "v0.107.78")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].ID != 7 || lists[0].RulesCount != 321 || lists[0].LastUpdated == nil || lists[1].Enabled {
		t.Fatalf("metadata was not retained: %#v", lists)
	}
}

func TestReadAllowlistsUsesAllowlistSemanticsWithoutBlocklistContamination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/filtering/status" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"filters":[{"id":7,"enabled":true,"url":"https://filters.test/list.txt","name":"Block","rules_count":321},{"id":8,"enabled":true,"url":"https://legacy-allow.test/list.txt","name":"Legacy allow","rules_count":12,"whitelist":true}],"whitelist_filters":[{"id":9,"enabled":false,"url":"https://allow.test/list.txt","name":"Allow","rules_count":22,"last_updated":"2026-08-01T01:02:03Z"}]}`))
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	lists, err := adapter.ReadAllowlists(context.Background(), probeRequest(server.URL), "v0.107.78")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].ID != 8 || lists[1].ID != 9 || lists[1].RulesCount != 22 || lists[1].LastUpdated == nil {
		t.Fatalf("allowlist metadata was not retained or categories crossed: %#v", lists)
	}
}

func TestValidateListenerStatusRejectsIncompleteIdentity(t *testing.T) {
	protectionEnabled := true
	disabledDuration := int64(0)
	for name, status := range map[string]statusResponse{
		"missing":         {},
		"invalid port":    {DNSAddresses: []string{"0.0.0.0"}, DNSPort: 0, ProtectionEnabled: &protectionEnabled, ProtectionDisabledDurationMS: &disabledDuration},
		"invalid address": {DNSAddresses: []string{"not-an-address"}, DNSPort: 53, ProtectionEnabled: &protectionEnabled, ProtectionDisabledDurationMS: &disabledDuration},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateListenerStatus(status); err == nil {
				t.Fatal("validateListenerStatus() error = nil")
			}
		})
	}
	if err := validateListenerStatus(statusResponse{DNSAddresses: []string{"0.0.0.0", "::"}, DNSPort: 53, ProtectionEnabled: &protectionEnabled, ProtectionDisabledDurationMS: &disabledDuration}); err != nil {
		t.Fatalf("valid listener status rejected: %v", err)
	}
}

func TestApplyConfigurationUsesSupportedEndpointsAndPreservesWhitelistFilters(t *testing.T) {
	requests := map[string][]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "admin" || password != "secret" {
			t.Errorf("basic authentication was not preserved")
		}
		if request.Method == http.MethodGet && request.URL.Path == "/control/filtering/status" {
			_ = json.NewEncoder(response).Encode(map[string]any{"enabled": true, "interval": 24, "filters": []map[string]any{
				{"name": "wanted", "url": "https://example.test/wanted.txt", "enabled": false, "whitelist": false},
				{"name": "extra", "url": "https://example.test/extra.txt", "enabled": true, "whitelist": false},
				{"name": "allow", "url": "https://example.test/allow.txt", "enabled": true, "whitelist": true},
			}})
			return
		}
		if request.Method != http.MethodPost {
			http.Error(response, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests[request.URL.Path] = append(requests[request.URL.Path], body)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	desired := configuration.Document{SchemaVersion: 1, Shared: configuration.Shared{
		DNS:       configuration.DNS{UpstreamDNS: []string{"1.1.1.1"}},
		Filtering: configuration.Filtering{Enabled: true, UpdateInterval: 24, FilterURLs: []string{"https://example.test/wanted.txt"}, UserRules: []string{"||ads.test^"}},
	}}
	err := adapter.ApplyConfiguration(context.Background(), domain.NodeProbeRequest{BaseURL: server.URL, CertificatePolicy: domain.CertificateInsecureHTTP, Credentials: domain.NodeCredentials{Username: "admin", Password: "secret"}}, desired)
	if err != nil {
		t.Fatalf("ApplyConfiguration() error = %v", err)
	}
	for _, path := range []string{"/control/dns_config", "/control/filtering/config", "/control/filtering/set_rules"} {
		if len(requests[path]) != 1 {
			t.Fatalf("%s requests = %d, want 1", path, len(requests[path]))
		}
	}
	setURLs := requests["/control/filtering/set_url"]
	if len(setURLs) != 2 {
		t.Fatalf("set_url requests = %d, want enable and disable", len(setURLs))
	}
	for _, payload := range setURLs {
		if payload["url"] == "https://example.test/allow.txt" {
			t.Fatal("whitelist filter was mutated")
		}
	}
}

func TestAllowlistReconciliationUsesWhitelistFlagAndPreservesBlocklists(t *testing.T) {
	type recordedRequest struct {
		path string
		body map[string]any
	}
	requests := []recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if (request.URL.Path != "/control/filtering/set_url" && request.URL.Path != "/control/filtering/add_url") || request.Method != http.MethodPost {
			http.NotFound(response, request)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode set_url request: %v", err)
			return
		}
		requests = append(requests, recordedRequest{path: request.URL.Path, body: body})
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	current := filterStatusResponse{
		Filters: []filterListResponse{
			{Name: "Block", URL: "https://filters.test/list.txt", Enabled: true},
			{Name: "Legacy allow", URL: "https://allow.test/wanted.txt", Enabled: false, Whitelist: true},
		},
		WhitelistFilters: []filterListResponse{{Name: "Old allow", URL: "https://allow.test/old.txt", Enabled: true}},
	}
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	if err := adapter.reconcileFilterURLs(context.Background(), probeRequest(server.URL), current, []string{"https://allow.test/wanted.txt", "https://allow.test/new.txt"}, true); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("filter URL requests = %d, want enable, add, and disable: %#v", len(requests), requests)
	}
	foundAdd := false
	for _, request := range requests {
		if request.body["whitelist"] != true || request.body["url"] == "https://filters.test/list.txt" {
			t.Fatalf("allowlist flag or category isolation failed: %#v", requests)
		}
		if request.path == "/control/filtering/add_url" && request.body["url"] == "https://allow.test/new.txt" {
			foundAdd = true
		}
	}
	if !foundAdd {
		t.Fatalf("allowlist add_url request missing or incorrect: %#v", requests)
	}
}

func TestReadConfigurationKeepsV010752OnFrozenSchemaV1(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/control/status":
			_, _ = response.Write([]byte(`{"version":"v0.107.52","running":true,"dns_addresses":["0.0.0.0"],"dns_port":53,"protection_enabled":true,"protection_disabled_duration":0}`))
		case "/control/dns_info":
			_, _ = response.Write([]byte(`{"upstream_dns":["1.1.1.1"],"protection_enabled":true,"protection_disabled_until":null}`))
		case "/control/filtering/status":
			_, _ = response.Write([]byte(`{"enabled":true,"interval":24,"filters":[],"user_rules":[]}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	document, profile, err := adapter.ReadConfiguration(context.Background(), probeRequest(server.URL), "v0.107.52")
	if err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != configuration.LegacySchemaVersion || profile.SchemaVersion != configuration.LegacySchemaVersion || !profile.Features["querylog_clear"] || !profile.Features["stats_reset"] || profile.Features["query_log"] || profile.Features["statistics"] || len(requests) != 3 {
		t.Fatalf("legacy inventory document=%#v profile=%#v requests=%#v", document, profile, requests)
	}
}

func TestReadConfigurationV2SupportsTestedMixedAndProvisionallyCompatiblePatches(t *testing.T) {
	responses := map[string]string{
		"/control/status":               `{"version":"v0.107.78","running":true,"dns_addresses":["0.0.0.0"],"dns_port":53,"protection_enabled":true,"protection_disabled_duration":0}`,
		"/control/dns_info":             `{"upstream_dns":["1.1.1.1"],"protection_enabled":true,"protection_disabled_until":null,"cache_size":4096,"cache_enabled":false,"upstream_timeout":12,"upstream_mode":"parallel"}`,
		"/control/filtering/status":     `{"enabled":true,"interval":24,"filters":[],"whitelist_filters":[{"enabled":true,"url":"https://allow.test/list.txt","name":"allow"}],"user_rules":["||ads.test^"]}`,
		"/control/clients":              `{"clients":[{"name":"printer","ids":["192.0.2.10"],"use_global_settings":true,"safe_search":{"enabled":false},"blocked_services_schedule":{"time_zone":"Local"}}]}`,
		"/control/rewrite/list":         `[{"domain":"router.test","answer":"192.0.2.1","enabled":false}]`,
		"/control/rewrite/settings":     `{"enabled":false}`,
		"/control/blocked_services/get": `{"ids":["youtube"],"schedule":{"time_zone":"Pacific/Auckland"}}`,
		"/control/safebrowsing/status":  `{"enabled":true}`,
		"/control/parental/status":      `{"enabled":false}`,
		"/control/safesearch/status":    `{"enabled":true,"google":true,"ecosia":true}`,
		"/control/querylog/config":      `{"enabled":true,"interval":604800000,"anonymize_client_ip":true,"ignored":["health.test"],"ignored_enabled":false}`,
		"/control/stats/config":         `{"enabled":true,"interval":2592000000,"ignored":[],"ignored_enabled":true}`,
		"/control/tls/status":           `{"enabled":true,"server_name":"dns.test","valid_pair":true,"certificate_chain":"SECRET CERT","private_key":"SECRET KEY"}`,
		"/control/dhcp/status":          `{"enabled":true,"interface_name":"eth0","v4":{"gateway_ip":"192.0.2.1","subnet_mask":"255.255.255.0","range_start":"192.0.2.100","range_end":"192.0.2.200","lease_duration":3600},"v6":{},"leases":[],"static_leases":[{"mac":"00:11:22:33:44:55","ip":"192.0.2.10","hostname":"printer"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, ok := responses[request.URL.Path]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(body))
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	for _, version := range []string{"v0.107.78", "v0.107.79", "v0.107.80"} {
		responses["/control/status"] = strings.Replace(`{"version":"v0.107.78","running":true,"dns_addresses":["0.0.0.0"],"dns_port":53,"protection_enabled":true,"protection_disabled_duration":0,"future_additive_field":true}`, "v0.107.78", version, 1)
		document, profile, err := adapter.ReadConfiguration(context.Background(), probeRequest(server.URL), version)
		if err != nil {
			t.Fatalf("ReadConfiguration(%s) error = %v", version, err)
		}
		if document.SchemaVersion != configuration.SchemaVersion || !profile.Features["dhcp"] || !profile.Features["filter_interval_arbitrary"] || len(document.Shared.Clients) != 1 || len(document.Shared.Rewrites) != 1 || document.Shared.RewritesEnabled || document.Shared.Rewrites[0].Enabled || document.Shared.DNS.CacheEnabled || document.Shared.DNS.UpstreamTimeout != 12 || document.Shared.QueryLog.IgnoredEnabled || document.NodeSpecific.DHCP == nil || !document.ObservedOnly.TLS.ValidPair {
			t.Fatalf("broader inventory for %s was incomplete: document=%#v profile=%#v", version, document, profile)
		}
		if provisional := len(profile.Warnings) > 0 && strings.Contains(strings.Join(profile.Warnings, " "), "provisionally compatible"); provisional != (version == "v0.107.80") {
			t.Fatalf("provisional warning for %s = %v; warnings=%#v", version, provisional, profile.Warnings)
		}
		body, _, err := configuration.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "SECRET") {
			t.Fatalf("TLS secret entered canonical inventory: %s", body)
		}
	}
}

func TestCompatibilityFixturesNormalizeProtectionSemanticsAndAdditiveFields(t *testing.T) {
	for _, version := range []string{"v0.107.78", "v0.107.79"} {
		var status statusResponse
		readFixture(t, filepath.Join("testdata", version, "status.json"), &status)
		if err := validateListenerStatus(status); err != nil {
			t.Fatalf("%s status fixture: %v", version, err)
		}
		var dns dnsInfoResponse
		readFixture(t, filepath.Join("testdata", version, "dns_info.json"), &dns)
		if err := validateDNSProtection(dns); err != nil {
			t.Fatalf("%s DNS fixture: %v", version, err)
		}
		document := configurationDocument(version, status, dns, filterStatusResponse{})
		if version == "v0.107.78" && (!document.Shared.DNS.ProtectionEnabled || document.ObservedOnly.ProtectionDisabledUntil != "") {
			t.Fatalf("unexpected .78 protection state: %#v", document)
		}
		if version == "v0.107.79" && (document.Shared.DNS.ProtectionEnabled || document.ObservedOnly.ProtectionDisabledUntil != "2026-08-19T00:01:00Z") {
			t.Fatalf("unexpected .79 paused protection state: %#v", document)
		}
	}
}

func TestProtectionSemanticValidationRejectsMissingWrongTypeAndContradiction(t *testing.T) {
	for name, body := range map[string]string{
		"missing":       `{"upstream_dns":[]}`,
		"wrong type":    `{"protection_enabled":"yes"}`,
		"contradiction": `{"protection_enabled":true,"protection_disabled_until":"2026-08-19T00:01:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var dns dnsInfoResponse
			if err := json.Unmarshal([]byte(body), &dns); err != nil {
				if name == "wrong type" {
					return
				}
				t.Fatal(err)
			}
			if err := validateDNSProtection(dns); err == nil {
				t.Fatal("invalid protection semantics were accepted")
			}
		})
	}
}

func TestDNSOperationalCommandsMapSafeResultsAndExactPaths(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/control/test_upstream_dns":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(payload, map[string]any{
				"upstream_dns":     []any{"https://user:secret@dns.example/dns-query", "192.0.2.53"},
				"bootstrap_dns":    []any{"9.9.9.9"},
				"fallback_dns":     []any{},
				"private_upstream": []any{"192.0.2.1"},
			}) {
				t.Fatalf("payload=%#v", payload)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"https://user:xxxxx@dns.example:443/dns-query":"OK","192.0.2.53":"dial detail that must not escape","9.9.9.9":"OK","192.0.2.1":"OK"}`))
		case "/control/cache_clear":
			response.WriteHeader(http.StatusOK)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	results, err := adapter.TestUpstreamDNS(context.Background(), probeRequest(server.URL), operations.UpstreamInput{
		DraftVersion: 4, UpstreamDNS: []string{"https://user:secret@dns.example/dns-query", "192.0.2.53"}, BootstrapDNS: []string{"9.9.9.9"}, FallbackDNS: []string{}, PrivateReverseDNS: []string{"192.0.2.1"}, UpstreamMode: "parallel", UsePrivateReverseResolvers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ResolverID != "upstream-1" || results[0].Status != "succeeded" || results[1].ErrorCode != "UPSTREAM_TEST_FAILED" {
		t.Fatalf("results=%#v", results)
	}
	encoded, _ := json.Marshal(results)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "dial detail") {
		t.Fatalf("unsafe result=%s", encoded)
	}
	if err := adapter.ClearDNSCache(context.Background(), probeRequest(server.URL)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requests, []string{"POST /control/test_upstream_dns", "POST /control/cache_clear"}) {
		t.Fatalf("requests=%#v", requests)
	}
}

func TestHostFilteringOperationalCommandMapsAttributedSafeResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/control/filtering/check_host" || request.URL.Query().Get("name") != "ads.example" || request.URL.Query().Get("client") != "192.0.2.10" || request.URL.Query().Get("qtype") != "AAAA" {
			t.Fatalf("request=%s %s query=%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"reason":"FilteredBlackList","rules":[{"text":"||ads.example^","filter_list_id":42}],"service_name":"tracking","cname":"safe.example","ip_addrs":["192.0.2.44"]}`))
	}))
	defer server.Close()
	result, err := NewConfigurationReader(NewProbe(time.Second)).TestHostFiltering(context.Background(), probeRequest(server.URL), operations.HostFilterInput{Hostname: "ads.example", Client: "192.0.2.10", QueryType: "AAAA"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.Reason != "FilteredBlackList" || len(result.Rules) != 1 || result.Rules[0].Text != "||ads.example^" || result.Rules[0].FilterListID != 42 || result.CanonicalName != "safe.example" {
		t.Fatalf("result=%#v", result)
	}
}

func TestPolicyOperationalCommandsUseExactNoBodyPaths(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != 0 {
			t.Fatalf("unexpected body=%q", body)
		}
		requests = append(requests, request.Method+" "+request.URL.Path)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(time.Second))
	if err := adapter.ClearQueryLog(context.Background(), probeRequest(server.URL)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ResetStatistics(context.Background(), probeRequest(server.URL)); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /control/querylog_clear", "POST /control/stats_reset"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests=%#v want=%#v", requests, want)
	}
}

func TestPolicyOperationalCommandsMapSafeFailureAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		delay   time.Duration
		timeout time.Duration
		want    domain.ErrorKind
	}{
		{name: "authentication", status: http.StatusUnauthorized, want: domain.ErrorNodeAuth},
		{name: "rejected", status: http.StatusInternalServerError, want: domain.ErrorNodeApply},
		{name: "timeout", status: http.StatusOK, delay: 50 * time.Millisecond, timeout: time.Millisecond, want: domain.ErrorNodeUnreachable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				if test.delay > 0 {
					time.Sleep(test.delay)
				}
				response.WriteHeader(test.status)
			}))
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			err := NewConfigurationReader(NewProbe(timeout)).ClearQueryLog(context.Background(), probeRequest(server.URL))
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Kind != test.want {
				t.Fatalf("error=%#v want=%s", err, test.want)
			}
		})
	}
}

func TestHostFilteringOperationalCommandMapsLegacyRuleAndSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		body     string
		delay    time.Duration
		timeout  time.Duration
		wantRule bool
		wantKind domain.ErrorKind
	}{
		{name: "legacy response", status: http.StatusOK, body: `{"reason":"FilteredBlackList","rule":"||legacy.example^","filter_id":7}`, wantRule: true},
		{name: "capability", status: http.StatusNotFound, wantKind: domain.ErrorCapability},
		{name: "invalid json", status: http.StatusOK, body: `{`, wantKind: domain.ErrorNodeResponse},
		{name: "unsafe rule", status: http.StatusOK, body: `{"rules":[{"text":"unsafe\nrule","filter_list_id":1}]}`, wantKind: domain.ErrorNodeResponse},
		{name: "invalid address", status: http.StatusOK, body: `{"ip_addrs":["not-an-address"]}`, wantKind: domain.ErrorNodeResponse},
		{name: "timeout", status: http.StatusOK, body: `{}`, delay: 50 * time.Millisecond, timeout: time.Millisecond, wantKind: domain.ErrorNodeUnreachable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				if test.delay > 0 {
					time.Sleep(test.delay)
				}
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			result, err := NewConfigurationReader(NewProbe(timeout)).TestHostFiltering(context.Background(), probeRequest(server.URL), operations.HostFilterInput{Hostname: "legacy.example"})
			if test.wantRule {
				if err != nil || len(result.Rules) != 1 || result.Rules[0].Text != "||legacy.example^" {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				return
			}
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Kind != test.wantKind {
				t.Fatalf("error=%#v want=%s", err, test.wantKind)
			}
		})
	}
}

func TestOperationalUpstreamStatusMatchesAdGuardCanonicalAddresses(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested string
		returned  string
	}{
		{name: "plain ipv4", requested: "192.168.5.3", returned: "192.168.5.3:53"},
		{name: "plain ipv6", requested: "2a10:50c0::1:ff", returned: "[2a10:50c0::1:ff]:53"},
		{name: "plain explicit port", requested: "94.140.14.140:53", returned: "94.140.14.140:53"},
		{name: "udp hostname", requested: "udp://unfiltered.adguard-dns.com", returned: "unfiltered.adguard-dns.com:53"},
		{name: "tcp", requested: "tcp://94.140.14.140", returned: "tcp://94.140.14.140:53"},
		{name: "tcp ipv6", requested: "tcp://[2a10:50c0::1:ff]", returned: "tcp://[2a10:50c0::1:ff]:53"},
		{name: "tls", requested: "tls://unfiltered.adguard-dns.com", returned: "tls://unfiltered.adguard-dns.com:853"},
		{name: "quad9 doh", requested: "https://dns10.quad9.net/dns-query", returned: "https://dns10.quad9.net:443/dns-query"},
		{name: "cloudflare gateway doh", requested: "https://ky7ror94zq.cloudflare-gateway.com/dns-query", returned: "https://ky7ror94zq.cloudflare-gateway.com:443/dns-query"},
		{name: "http3", requested: "h3://unfiltered.adguard-dns.com/dns-query", returned: "https://unfiltered.adguard-dns.com:443/dns-query"},
		{name: "quic", requested: "quic://unfiltered.adguard-dns.com", returned: "quic://unfiltered.adguard-dns.com:853"},
		{name: "stamp", requested: "sdns://AQcAAAAAAAAAEzE5Mi4wLjIuMTo1NDQz", returned: "sdns://AQcAAAAAAAAAEzE5Mi4wLjIuMTo1NDQz"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, ok := operationalUpstreamStatus(map[string]string{test.returned: "OK"}, test.requested)
			if !ok || status != "OK" {
				t.Fatalf("status=%q ok=%t", status, ok)
			}
		})
	}
}

func TestOperationalUpstreamStatusesMatchDomainSpecificAndExpandedStampResults(t *testing.T) {
	statuses, ok := operationalUpstreamStatuses(map[string]string{
		"94.140.14.140:53":     "OK",
		"[2a10:50c0::1:ff]:53": "OK",
	}, "[/example.local/]94.140.14.140 2a10:50c0::1:ff")
	if !ok || len(statuses) != 2 || statuses[0] != "OK" || statuses[1] != "OK" {
		t.Fatalf("statuses=%#v ok=%t", statuses, ok)
	}

	statuses, ok = operationalUpstreamStatuses(map[string]string{"https://stamp.example:443/dns-query": "OK"}, "sdns://opaque-stamp")
	if !ok || len(statuses) != 1 || statuses[0] != "OK" {
		t.Fatalf("stamp statuses=%#v ok=%t", statuses, ok)
	}
}

func TestUpstreamOperationalCommandMapsCapabilityInvalidAndTimeoutErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body       string
		delay      time.Duration
		timeout    time.Duration
		wantedKind domain.ErrorKind
	}{
		{name: "capability", status: http.StatusNotFound, wantedKind: domain.ErrorCapability},
		{name: "invalid response", status: http.StatusOK, body: `{`, wantedKind: domain.ErrorNodeResponse},
		{name: "timeout", status: http.StatusOK, body: `{}`, delay: 50 * time.Millisecond, timeout: time.Millisecond, wantedKind: domain.ErrorNodeUnreachable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				if test.delay > 0 {
					time.Sleep(test.delay)
				}
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			_, err := NewConfigurationReader(NewProbe(timeout)).TestUpstreamDNS(context.Background(), probeRequest(server.URL), operations.UpstreamInput{DraftVersion: 1, UpstreamDNS: []string{"1.1.1.1"}, UpstreamMode: "load_balance"})
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Kind != test.wantedKind {
				t.Fatalf("error=%#v want kind=%s", err, test.wantedKind)
			}
		})
	}
}

func TestApplyConfigurationV2UsesDocumentedMethods(t *testing.T) {
	methods := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		methods[request.URL.Path] = request.Method
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			switch request.URL.Path {
			case "/control/filtering/status":
				_, _ = response.Write([]byte(`{"enabled":true,"filters":[],"whitelist_filters":[]}`))
			case "/control/dns_info":
				_, _ = response.Write([]byte(`{"cache_enabled":true,"upstream_timeout":10}`))
			case "/control/clients":
				_, _ = response.Write([]byte(`{"clients":[]}`))
			case "/control/rewrite/list":
				_, _ = response.Write([]byte(`[]`))
			case "/control/rewrite/settings":
				_, _ = response.Write([]byte(`{"enabled":true}`))
			case "/control/querylog/config", "/control/stats/config":
				_, _ = response.Write([]byte(`{"enabled":true,"interval":86400000,"ignored":[],"ignored_enabled":true}`))
			case "/control/dhcp/status":
				_, _ = response.Write([]byte(`{"static_leases":[]}`))
			default:
				http.NotFound(response, request)
			}
			return
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	desired := configuration.Document{SchemaVersion: configuration.SchemaVersion, Shared: configuration.Shared{Filtering: configuration.Filtering{UpdateInterval: 24}, Services: configuration.Services{BlockedSchedule: configuration.Schedule{TimeZone: "Local", Days: map[string]configuration.DayRange{}}}}, NodeSpecific: configuration.NodeSpecific{DHCP: &configuration.DHCPConfig{Enabled: true, InterfaceName: "eth0", IPv4: configuration.DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.2.100", RangeEnd: "192.0.2.200", LeaseDuration: 3600}}}}
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	if err := adapter.ApplyConfiguration(context.Background(), probeRequest(server.URL), desired); err != nil {
		t.Fatal(err)
	}
	for path, method := range map[string]string{
		"/control/blocked_services/update": http.MethodPut,
		"/control/safesearch/settings":     http.MethodPut,
		"/control/querylog/config/update":  http.MethodPut,
		"/control/stats/config/update":     http.MethodPut,
		"/control/dhcp/set_config":         http.MethodPost,
		"/control/dns_config":              http.MethodPost,
		"/control/rewrite/settings/update": http.MethodPut,
	} {
		if methods[path] != method {
			t.Errorf("%s method = %q, want %q", path, methods[path], method)
		}
	}
}

func TestReconcileDHCPSkipsConfigurationWriteWhenAlreadyConverged(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if request.Method == http.MethodGet && request.URL.Path == "/control/dhcp/status" {
			_, _ = response.Write([]byte(`{"enabled":false,"interface_name":"","v4":{"gateway_ip":"","subnet_mask":"","range_start":"","range_end":"","lease_duration":0},"v6":{"range_start":"","lease_duration":0},"static_leases":[]}`))
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/control/dhcp/add_static_lease" {
			response.WriteHeader(http.StatusOK)
			return
		}
		http.Error(response, "unexpected mutation", http.StatusBadRequest)
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(time.Second))
	desired := configuration.DHCPConfig{StaticLeases: []configuration.DHCPStaticLease{{MAC: "00:11:22:33:44:55", IP: "192.0.2.10", Hostname: "printer"}}}
	if err := adapter.reconcileDHCP(context.Background(), probeRequest(server.URL), desired); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /control/dhcp/status", "POST /control/dhcp/add_static_lease"}
	if len(requests) != len(want) || requests[0] != want[0] || requests[1] != want[1] {
		t.Fatalf("requests = %#v, want %#v without set_config", requests, want)
	}
}

func TestMutationFailureIncludesSafeOperationAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "rejected payload containing secret-value", http.StatusBadRequest)
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(time.Second))
	err := adapter.post(context.Background(), probeRequest(server.URL), "/control/dhcp/set_config", map[string]any{"enabled": false})
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorNodeApply {
		t.Fatalf("error = %#v, want NODE_APPLY_FAILED", err)
	}
	if !strings.Contains(domainError.Message, "POST /control/dhcp/set_config") || !strings.Contains(domainError.Message, "HTTP 400") {
		t.Fatalf("safe operation detail missing from %q", domainError.Message)
	}
	if strings.Contains(domainError.Message, "secret-value") {
		t.Fatalf("node response body leaked into error: %q", domainError.Message)
	}
}

func TestOptionalDHCPReadOnlyTreatsUnavailableStatusesAsUnsupported(t *testing.T) {
	for name, status := range map[string]int{"not found": http.StatusNotFound, "platform unsupported": http.StatusInternalServerError, "not implemented": http.StatusNotImplemented} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(status)
			}))
			defer server.Close()
			adapter := NewConfigurationReader(NewProbe(2 * time.Second))
			supported, err := adapter.getOptional(context.Background(), probeRequest(server.URL), "/control/dhcp/status", &dhcpStatusResponse{})
			if err != nil || supported {
				t.Fatalf("getOptional() supported=%t err=%v", supported, err)
			}
		})
	}
}

func TestOptionalDHCPReadRejectsMalformedSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"enabled":`))
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(2 * time.Second))
	if supported, err := adapter.getOptional(context.Background(), probeRequest(server.URL), "/control/dhcp/status", &dhcpStatusResponse{}); err == nil || supported {
		t.Fatalf("malformed optional response supported=%t err=%v", supported, err)
	}
}

func TestReadDHCPInterfacesMapsExactAdGuardShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/control/dhcp/interfaces" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"eth0":{"name":"eth0","hardware_address":"00:11:22:33:44:55","ipv4_addresses":["192.0.2.2"],"ipv6_addresses":["2001:db8::2"],"gateway_ip":"192.0.2.1","flags":"up|broadcast|multicast"}}`))
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(time.Second))
	interfaces, err := adapter.ReadDHCPInterfaces(context.Background(), probeRequest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 1 || interfaces[0].Name != "eth0" || interfaces[0].GatewayIP != "192.0.2.1" || len(interfaces[0].Flags) != 3 || interfaces[0].IPv6Addresses[0] != "2001:db8::2" {
		t.Fatalf("interfaces = %#v", interfaces)
	}
}

func TestReadDHCPInterfacesReportsUnavailableEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusNotImplemented} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(status) }))
			defer server.Close()
			adapter := NewConfigurationReader(NewProbe(time.Second))
			_, err := adapter.ReadDHCPInterfaces(context.Background(), probeRequest(server.URL))
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorCapability {
				t.Fatalf("error = %#v, want capability error", err)
			}
		})
	}
}

func TestFindActiveDHCPUsesJSONRequestAndSanitisesNodeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/control/dhcp/find_active_dhcp" {
			http.NotFound(response, request)
			return
		}
		var input map[string]string
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input["interface"] != "eth0" {
			http.Error(response, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = response.Write([]byte(`{"v4":{"other_server":{"found":"error","error":"private implementation detail"},"static_ip":{"static":"no","ip":"192.0.2.2"}},"v6":{"other_server":{"found":"yes"}}}`))
	}))
	defer server.Close()

	adapter := NewConfigurationReader(NewProbe(time.Second))
	result, err := adapter.FindActiveDHCP(context.Background(), probeRequest(server.URL), "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if result.IPv4OtherServer.Status != "error" || strings.Contains(result.IPv4OtherServer.Message, "private") || result.IPv4StaticIP.IP != "192.0.2.2" || result.IPv6OtherServer.Status != "yes" {
		t.Fatalf("result = %#v", result)
	}
}

func TestFindActiveDHCPTimeoutIsNodeUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = response.Write([]byte(`{}`))
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(5 * time.Millisecond))
	_, err := adapter.FindActiveDHCP(context.Background(), probeRequest(server.URL), "eth0")
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorNodeUnreachable {
		t.Fatalf("error = %#v, want node unreachable", err)
	}
}

func TestDHCPResetOperationsUseExactNoBodyEndpoints(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		if len(body) != 0 || request.Header.Get("Content-Type") != "" {
			t.Errorf("reset request unexpectedly had body=%q content-type=%q", body, request.Header.Get("Content-Type"))
		}
		paths = append(paths, request.URL.Path)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	adapter := NewConfigurationReader(NewProbe(time.Second))
	request := probeRequest(server.URL)
	if err := adapter.ResetDHCPLeases(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ResetDHCPConfiguration(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/control/dhcp/reset_leases" || paths[1] != "/control/dhcp/reset" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestDHCPResetFailureAndTimeoutAreSanitised(t *testing.T) {
	t.Run("upstream body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "credential=secret private response", http.StatusInternalServerError)
		}))
		defer server.Close()
		adapter := NewConfigurationReader(NewProbe(time.Second))
		err := adapter.ResetDHCPConfiguration(context.Background(), probeRequest(server.URL))
		var domainError *domain.Error
		if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorNodeApply || strings.Contains(err.Error(), "credential") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe error = %#v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			response.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		adapter := NewConfigurationReader(NewProbe(5 * time.Millisecond))
		err := adapter.ResetDHCPLeases(context.Background(), probeRequest(server.URL))
		var domainError *domain.Error
		if !errors.As(err, &domainError) || domainError.Kind != domain.ErrorNodeUnreachable {
			t.Fatalf("error = %#v, want node unreachable", err)
		}
	})
}

func TestReadBlockedServicesCatalogueSupportsUngroupedAndGroupedContracts(t *testing.T) {
	tests := []struct {
		name, version, body string
		wantGroups          int
		wantGroupID         string
	}{
		{
			name: "frozen schema catalogue", version: "v0.107.52",
			body: `{"blocked_services":[{"id":"youtube","name":"YouTube","rules":["||youtube.com^"],"icon_svg":"PHN2Zy8+"}]}`,
		},
		{
			name: "pre-group catalogue", version: "v0.107.61",
			body: `{"blocked_services":[{"id":"youtube","name":"YouTube","rules":["||youtube.com^"],"icon_svg":"PHN2Zy8+"}]}`,
		},
		{
			name: "grouped catalogue", version: "v0.107.78",
			body:       `{"blocked_services":[{"id":"youtube","name":"YouTube","rules":["||youtube.com^"],"icon_svg":"PHN2Zy8+","group_id":"streaming"}],"groups":[{"id":"streaming"}]}`,
			wantGroups: 1, wantGroupID: "streaming",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var method, path string
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				method, path = request.Method, request.URL.Path
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()

			reader := NewConfigurationReader(NewProbe(time.Second))
			catalogue, err := reader.ReadBlockedServicesCatalogue(context.Background(), probeRequest(server.URL), test.version)
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodGet || path != "/control/blocked_services/all" {
				t.Fatalf("unexpected request %s %s", method, path)
			}
			if len(catalogue.Services) != 1 || catalogue.Services[0].ID != "youtube" || catalogue.Services[0].Name != "YouTube" || catalogue.Services[0].GroupID != test.wantGroupID || len(catalogue.Groups) != test.wantGroups {
				t.Fatalf("unexpected catalogue: %#v", catalogue)
			}
		})
	}
}

func TestReadBlockedServicesCatalogueRejectsInvalidMetadataAndUnsupportedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"blocked_services":[{"id":"","name":"Missing ID"}]}`))
	}))
	defer server.Close()
	reader := NewConfigurationReader(NewProbe(time.Second))
	request := probeRequest(server.URL)
	if _, err := reader.ReadBlockedServicesCatalogue(context.Background(), request, "v0.107.78"); err == nil {
		t.Fatal("invalid catalogue metadata was accepted")
	}
	if _, err := reader.ReadBlockedServicesCatalogue(context.Background(), request, "v0.108.0"); err == nil {
		t.Fatal("unsupported version was accepted")
	}
}

func probeRequest(baseURL string) domain.NodeProbeRequest {
	return domain.NodeProbeRequest{BaseURL: baseURL, CertificatePolicy: domain.CertificateInsecureHTTP, Credentials: domain.NodeCredentials{Username: "admin", Password: "secret"}}
}

func readFixture(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatal(err)
	}
}
