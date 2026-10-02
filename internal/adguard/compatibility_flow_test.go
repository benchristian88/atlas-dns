package adguard

import (
	"context"
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
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

// Only the storage boundary is faked; probe, adapter, canonicalization,
// observation, import validation, draft creation, and audit creation are real.
type compatibilityRepository struct {
	inventory.Repository
	node     domain.Node
	snapshot inventory.Snapshot
	profile  inventory.CapabilityProfile
	draft    inventory.Draft
	audit    domain.AuditEvent
}

func (r *compatibilityRepository) NodeRecordByID(context.Context, string) (domain.NodeRecord, error) {
	return domain.NodeRecord{Node: r.node}, nil
}
func (r *compatibilityRepository) NodeByID(context.Context, string) (domain.Node, error) {
	return r.node, nil
}
func (r *compatibilityRepository) SaveObservation(_ context.Context, snapshot inventory.Snapshot, profile inventory.CapabilityProfile) error {
	r.snapshot, r.profile = snapshot, profile
	return nil
}
func (r *compatibilityRepository) SnapshotByID(context.Context, string) (inventory.Snapshot, error) {
	return r.snapshot, nil
}
func (r *compatibilityRepository) DraftByCluster(context.Context, string) (inventory.Draft, error) {
	return inventory.Draft{}, domain.NewError(domain.ErrorNotFound, "no draft")
}
func (r *compatibilityRepository) ImportDraft(_ context.Context, draft inventory.Draft, _ int, audit domain.AuditEvent) error {
	r.draft, r.audit = draft, audit
	return nil
}

type compatibilityCredentials struct{}

func (compatibilityCredentials) Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error) {
	return domain.NodeCredentials{Username: "admin", Password: "test-password"}, nil
}

func compatibilityFixtureServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	fixtureVersion := version
	if version == "v0.107.79" {
		fixtureVersion = "v1.0.0"
	}
	responses := map[string][]byte{}
	for _, name := range []string{"status", "dns_info", "filtering_status", "clients", "rewrite_list", "rewrite_settings", "blocked_services_get", "safebrowsing_status", "parental_status", "safesearch_status", "querylog_config", "stats_config", "tls_status", "dhcp_status"} {
		body, err := os.ReadFile(filepath.Join("testdata", fixtureVersion, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if name == "status" {
			body = []byte(strings.Replace(string(body), fixtureVersion, version, 1))
		}
		path := strings.ReplaceAll(name, "_", "/")
		if name == "dns_info" {
			path = name
		} else if name == "blocked_services_get" {
			path = "blocked_services/get"
		}
		responses["/control/"+path] = body
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "test-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := responses[r.URL.Path]
		if r.Method != http.MethodGet || !ok {
			t.Errorf("unexpected adapter call: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestCompatibleEncryptedDNSObservationAndImport(t *testing.T) {
	for _, version := range []string{"v0.107.79", "v1.0.0-b.1", "v1.0.0"} {
		t.Run(version, func(t *testing.T) {
			server := compatibilityFixtureServer(t, version)
			defer server.Close()
			probe := NewProbe(time.Second)
			credentials, _ := (compatibilityCredentials{}).Decrypt("", domain.EncryptedCredentials{})
			request := probeRequest(server.URL)
			request.Credentials = credentials
			result, err := probe.Status(context.Background(), request)
			if err != nil || result.Compatibility != domain.CompatibilitySupported || result.Version != version {
				t.Fatalf("status result=%+v error=%v", result, err)
			}
			const nodeID = "22222222-2222-4222-8222-222222222222"
			const clusterID = "11111111-1111-4111-8111-111111111111"
			repository := &compatibilityRepository{node: domain.Node{ID: nodeID, ClusterID: clusterID, Enabled: true, Version: result.Version, BaseURL: server.URL, CertificatePolicy: domain.CertificateInsecureHTTP}}
			service := inventory.NewService(repository, compatibilityCredentials{}, NewConfigurationReader(probe))
			snapshot, err := service.Observe(context.Background(), nodeID)
			if err != nil || snapshot.CollectionStatus != "succeeded" || snapshot.Document == nil || snapshot.CanonicalHash == "" {
				t.Fatalf("observation failed: snapshot=%+v error=%v", snapshot, err)
			}
			if !reflect.DeepEqual(snapshot.Document.NodeSpecific.BindHosts, []string{"192.168.1.100"}) {
				t.Fatalf("listener identity=%v", snapshot.Document.NodeSpecific.BindHosts)
			}
			if repository.profile.Compatibility != string(domain.CompatibilitySupported) || !repository.profile.Features["query_log"] || !repository.profile.Features["statistics_exact_range"] {
				t.Fatalf("incorrect capability profile: %+v", repository.profile)
			}
			draft, err := service.Import(context.Background(), domain.Actor{UserID: "33333333-3333-4333-8333-333333333333"}, clusterID, snapshot.ID, 0, true)
			if err != nil {
				t.Fatalf("import failed: %v", err)
			}
			if !reflect.DeepEqual(draft.Document.NodeOverrides[nodeID].BindHosts, []string{"192.168.1.100"}) || repository.audit.Action != "configuration.draft_imported" {
				t.Fatalf("import lost identity or audit: draft=%+v audit=%+v", draft, repository.audit)
			}
			if issues := configuration.ValidateDesired(draft.Document, []string{nodeID}); len(issues) != 0 {
				t.Fatalf("imported canonical document is invalid: %v", issues)
			}
			body, _, err := configuration.Marshal(*snapshot.Document)
			if err != nil || strings.Contains(string(body), "SECRET") {
				t.Fatalf("canonical inventory failed or retained TLS secrets: %v", err)
			}
		})
	}
}

func TestV1RecognitionDoesNotBypassResponseValidation(t *testing.T) {
	for _, version := range []string{"v1.0.0-b.1", "v1.0.0", "v1.25.7"} {
		for _, test := range []struct {
			name   string
			status int
			body   string
			kind   domain.ErrorKind
		}{
			{"authentication", http.StatusUnauthorized, "", domain.ErrorNodeAuth},
			{"HTTP failure", http.StatusInternalServerError, "", domain.ErrorNodeResponse},
			{"malformed JSON", http.StatusOK, "{", domain.ErrorNodeResponse},
			{"wrong type", http.StatusOK, `{"dns_addresses":42}`, domain.ErrorNodeResponse},
			{"missing required fields", http.StatusOK, `{}`, domain.ErrorNodeResponse},
			{"garbage metadata", http.StatusOK, `{"dns_addresses":["192.168.1.100","garbage://"],"dns_port":53,"protection_enabled":true,"protection_disabled_duration":0}`, domain.ErrorNodeResponse},
			{"encrypted only", http.StatusOK, `{"dns_addresses":["https://dns.example/dns-query"],"dns_port":53,"protection_enabled":true,"protection_disabled_duration":0}`, domain.ErrorNodeResponse},
			{"oversized response", http.StatusOK, strings.Repeat(" ", maxConfigurationBody+1), domain.ErrorNodeResponse},
		} {
			t.Run(version+"/"+test.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/control/status" {
						t.Errorf("invalid response allowed further reads: %s", r.URL.Path)
					}
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(test.body))
				}))
				defer server.Close()
				_, profile, err := NewConfigurationReader(NewProbe(time.Second)).ReadConfiguration(context.Background(), probeRequest(server.URL), version)
				if profile.Compatibility != string(domain.CompatibilitySupported) {
					t.Fatalf("recognized v1 version was not eligible: %+v", profile)
				}
				assertDomainErrorKind(t, err, test.kind)
			})
		}
	}
}
