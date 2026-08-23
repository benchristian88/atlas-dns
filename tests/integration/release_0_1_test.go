package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchristian88/atlas-dns/internal/adguard"
	controllerapi "github.com/benchristian88/atlas-dns/internal/api"
	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/database"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

func TestRelease01OperatorWorkflow(t *testing.T) {
	store := integrationStore(t)
	credentialCipher, err := auth.NewCredentialCipher(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager(bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(store, tokens, 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	probe := adguard.NewProbe(2 * time.Second)
	management := domain.NewManagementService(store, credentialCipher, probe)
	inventoryService := inventory.NewService(store, credentialCipher, adguard.NewConfigurationReader(probe))
	server := httptest.NewServer(controllerapi.NewServer(
		authService, management, inventoryService, store, store,
		slog.New(slog.NewTextHandler(io.Discard, nil)), false, "http://controller.example.test",
		30*time.Second, t.TempDir(),
	).Handler())
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	setupBody := `{"email":"admin@example.test","displayName":"Administrator","password":"correct horse battery staple"}`
	setupResponse := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/setup", setupBody, "")
	if setupResponse.StatusCode != http.StatusCreated {
		t.Fatalf("setup status = %d, body = %s", setupResponse.StatusCode, readBody(t, setupResponse))
	}
	_ = readBody(t, setupResponse)
	repeatedSetup := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/setup", setupBody, "")
	if repeatedSetup.StatusCode != http.StatusConflict {
		t.Fatalf("repeated setup status = %d, want 409; body = %s", repeatedSetup.StatusCode, readBody(t, repeatedSetup))
	}
	_ = readBody(t, repeatedSetup)
	csrf := cookieValue(jar.Cookies(baseURL), "atlas_dns_csrf")
	if csrf == "" || cookieValue(jar.Cookies(baseURL), "atlas_dns_session") == "" {
		t.Fatal("setup did not issue session and CSRF cookies")
	}

	clusterResponse := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/clusters", `{"name":"Home DNS","description":"Primary resolvers"}`, csrf)
	if clusterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create cluster status = %d, body = %s", clusterResponse.StatusCode, readBody(t, clusterResponse))
	}
	var cluster domain.Cluster
	decodeBody(t, clusterResponse, &cluster)

	nodeUsername := envOr("TEST_NODE_USERNAME", "agh-admin")
	nodePassword := envOr("TEST_NODE_PASSWORD", "node-secret-value")
	aghHandler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != nodeUsername || password != nodePassword {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(validAdGuardStatusResponse("v0.107.65")))
	})
	nodeAURL, nodeBURL := testNodeURLs(t, aghHandler)
	nodePayload := fmt.Sprintf(`{
		"name":"Node A","baseUrl":%q,"certificatePolicy":"insecure_http","enabled":true,
		"credentials":{"username":%q,"password":%q}
	}`, nodeAURL, nodeUsername, nodePassword)
	nodeResponse := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/clusters/"+cluster.ID+"/nodes", nodePayload, csrf)
	if nodeResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create node status = %d, body = %s", nodeResponse.StatusCode, readBody(t, nodeResponse))
	}
	nodeBody := readBody(t, nodeResponse)
	for _, secret := range []string{nodePassword, nodeUsername, "encryptedCredentials", "credentialNonce"} {
		if strings.Contains(nodeBody, secret) {
			t.Fatalf("node response exposed %q: %s", secret, nodeBody)
		}
	}
	var node domain.Node
	if err := json.Unmarshal([]byte(nodeBody), &node); err != nil {
		t.Fatal(err)
	}
	if node.HealthStatus != domain.NodeHealthy || node.Version != "v0.107.65" {
		t.Fatalf("node health = %q, version = %q", node.HealthStatus, node.Version)
	}
	testResponse := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/nodes/"+node.ID+"/test-connection", `{}`, csrf)
	if testResponse.StatusCode != http.StatusOK {
		t.Fatalf("test node status = %d, body = %s", testResponse.StatusCode, readBody(t, testResponse))
	}
	_ = readBody(t, testResponse)
	nodeBPayload := fmt.Sprintf(`{
		"name":"Node B","baseUrl":%q,"certificatePolicy":"insecure_http","enabled":true,
		"credentials":{"username":%q,"password":%q}
	}`, nodeBURL, nodeUsername, nodePassword)
	nodeBResponse := doJSON(t, client, http.MethodPost, server.URL+"/api/v1/clusters/"+cluster.ID+"/nodes", nodeBPayload, csrf)
	if nodeBResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create second node status = %d, body = %s", nodeBResponse.StatusCode, readBody(t, nodeBResponse))
	}
	_ = readBody(t, nodeBResponse)
	listResponse := doJSON(t, client, http.MethodGet, server.URL+"/api/v1/clusters/"+cluster.ID+"/nodes", "", "")
	if listResponse.StatusCode != http.StatusOK {
		t.Fatalf("list nodes status = %d, body = %s", listResponse.StatusCode, readBody(t, listResponse))
	}
	var nodeList struct {
		Items             []domain.Node `json:"items"`
		StaleAfterSeconds int64         `json:"staleAfterSeconds"`
	}
	decodeBody(t, listResponse, &nodeList)
	if len(nodeList.Items) != 2 {
		t.Fatalf("node count = %d, want 2", len(nodeList.Items))
	}
	if nodeList.StaleAfterSeconds != 90 {
		t.Fatalf("staleAfterSeconds = %d, want 90", nodeList.StaleAfterSeconds)
	}
	var ciphertext []byte
	if err := store.Pool().QueryRow(context.Background(), "SELECT encrypted_credentials FROM nodes WHERE id = $1", node.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(nodePassword)) {
		t.Fatal("database credential ciphertext contains plaintext password")
	}

	auditResponse := doJSON(t, client, http.MethodGet, server.URL+"/api/v1/audit-events?limit=100", "", "")
	if auditResponse.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d, body = %s", auditResponse.StatusCode, readBody(t, auditResponse))
	}
	auditBody := readBody(t, auditResponse)
	for _, action := range []string{"user.created", "auth.login.succeeded", "cluster.created", "node.created", "node.connection_tested"} {
		if !strings.Contains(auditBody, action) {
			t.Errorf("audit response does not include %q", action)
		}
	}
	if strings.Contains(auditBody, nodePassword) {
		t.Fatal("audit response exposed node password")
	}

	server.Close()
	statusRequest, err := http.NewRequest(http.MethodGet, nodeAURL+"/control/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	statusRequest.SetBasicAuth(nodeUsername, nodePassword)
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatalf("AdGuard test node stopped with controller: %v", err)
	}
	if statusResponse.StatusCode != http.StatusOK {
		t.Fatalf("AdGuard test node status after controller shutdown = %d, body = %s", statusResponse.StatusCode, readBody(t, statusResponse))
	}
	_ = readBody(t, statusResponse)
}

func testNodeURLs(t *testing.T, handler http.Handler) (string, string) {
	t.Helper()
	nodeAURL := strings.TrimSpace(os.Getenv("TEST_NODE_A_URL"))
	nodeBURL := strings.TrimSpace(os.Getenv("TEST_NODE_B_URL"))
	if (nodeAURL == "") != (nodeBURL == "") {
		t.Fatal("TEST_NODE_A_URL and TEST_NODE_B_URL must be supplied together")
	}
	if nodeAURL != "" {
		return nodeAURL, nodeBURL
	}
	nodeA := httptest.NewServer(handler)
	nodeB := httptest.NewServer(handler)
	t.Cleanup(nodeA.Close)
	t.Cleanup(nodeB.Close)
	return nodeA.URL, nodeB.URL
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func integrationStore(t *testing.T) *database.Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)
	id, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "atlas_dns_test_" + strings.ReplaceAll(id, "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") })
	testConfig := adminConfig.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	testPool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(testPool.Close)
	if err := database.ApplyMigrations(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	if err := database.RollbackLastMigration(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyMigrations(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	return database.NewStore(testPool)
}

func doJSON(t *testing.T, client *http.Client, method, target, body, csrf string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func decodeBody(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}
