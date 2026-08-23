package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/database"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
)

func TestRelease102MigratesV101NotificationDeliveriesInPlace(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	if err := database.RollbackLastMigration(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	const (
		clusterID  = "10210000-0000-4000-8000-000000000001"
		channelID  = "10210000-0000-4000-8000-000000000002"
		eventID    = "10210000-0000-4000-8000-000000000003"
		deliveryID = "10210000-0000-4000-8000-000000000004"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,created_at,updated_at) VALUES($1,'v1.0.1',$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO notification_channels
		(id,cluster_id,name,channel_type,enabled,destination_summary,encrypted_destination,destination_nonce,destination_key_version,destination_algorithm,record_version,created_at,updated_at)
		VALUES($1,$2,'Operations','webhook',true,'https://hooks.example.test',$3,$4,1,'AES-256-GCM',1,$5,$5)`, channelID, clusterID, []byte("ciphertext"), []byte("nonce"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO ha_operational_events
		(id,cluster_id,event_type,severity,summary,details_json,occurred_at)
		VALUES($1,$2,'dns.failed','critical','DNS failed','{}',$3)`, eventID, clusterID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO notification_deliveries
		(id,channel_id,event_id,status,attempt_count,error_code,created_at,completed_at,channel_name)
		VALUES($1,$2,$3,'failed',5,'NOTIFICATION_DELIVERY_REJECTED',$4,$4,'Operations')`, deliveryID, channelID, eventID, now); err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyMigrations(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	var version int
	var errorSummary string
	var httpStatus *int
	if err := store.Pool().QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT error_summary,http_status FROM notification_deliveries WHERE id=$1`, deliveryID).Scan(&errorSummary, &httpStatus); err != nil {
		t.Fatal(err)
	}
	if version != 15 || errorSummary != "" || httpStatus != nil {
		t.Fatalf("version=%d summary=%q status=%v", version, errorSummary, httpStatus)
	}
	items, err := store.ListHAHistory(ctx, haoperations.HistoryQuery{ClusterID: clusterID, Limit: 10})
	if err != nil || len(items) != 2 || items[0].Details == nil || items[1].Details == nil {
		t.Fatalf("migrated history=%#v err=%v", items, err)
	}
}

type transitionDNSProber struct {
	mu      sync.Mutex
	results []haoperations.DNSProbeResult
	index   int
}

func (p *transitionDNSProber) Probe(context.Context, haoperations.DNSProbeRequest) (haoperations.DNSProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := p.results[p.index]
	p.index++
	if result.Status == "healthy" {
		return result, nil
	}
	return result, errors.New("fake DNS probe failed")
}

func TestRelease102DNSTransitionsDeliverWebhookAndAppearInPaginatedHistory(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	const (
		userID    = "10200000-0000-4000-8000-000000000001"
		clusterID = "10200000-0000-4000-8000-000000000002"
		nodeID    = "10200000-0000-4000-8000-000000000003"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at)
		VALUES($1,'release-102@example.test','Administrator','hash','administrator',true,$2,$2)`, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,description,created_at,updated_at)
		VALUES($1,'Notification history','',$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO nodes
		(id,cluster_id,name,base_url,encrypted_credentials,credential_nonce,credential_key_version,credential_algorithm,certificate_policy,enabled,created_at,updated_at)
		VALUES($1,$2,'DNS 1','https://node.example.test',$3,$4,1,'AES-256-GCM','system',true,$5,$5)`, nodeID, clusterID, []byte("ciphertext"), []byte("nonce"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO node_lifecycle_settings
		(node_id,dns_probe_host,dns_probe_port,dns_probe_name,dns_probe_type,expected_rcode,probe_udp,probe_tcp,installation_type,created_at,updated_at)
		VALUES($1,'192.0.2.10',53,'.','NS',0,true,true,'docker',$2,$2)`, nodeID, now); err != nil {
		t.Fatal(err)
	}
	initial := haoperations.DNSProbeResult{ID: "10200000-0000-4000-8000-000000000004", ClusterID: clusterID, NodeID: nodeID, Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy", ProbedAt: now}
	if err := store.SaveDNSProbe(ctx, initial, nil); err != nil {
		t.Fatal(err)
	}

	var receiverMu sync.Mutex
	received := []map[string]any{}
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, "invalid", http.StatusBadRequest)
			return
		}
		receiverMu.Lock()
		received = append(received, payload)
		receiverMu.Unlock()
		response.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	cipher, err := auth.NewCredentialCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	notifications := haoperations.NewNotificationService(store, cipher, receiver.Client())
	actor := domain.Actor{UserID: userID, RequestID: "release-1.0.2"}
	channel, err := notifications.Create(ctx, actor, clusterID, "Operations", receiver.URL+"/private?token=hidden", true)
	if err != nil {
		t.Fatal(err)
	}

	prober := &transitionDNSProber{results: []haoperations.DNSProbeResult{
		{Status: "failed", UDPStatus: "failed", TCPStatus: "failed", ErrorCode: "DNS_PROBE_UNREACHABLE", ProbedAt: now.Add(time.Second)},
		{Status: "failed", UDPStatus: "failed", TCPStatus: "failed", ErrorCode: "DNS_PROBE_UNREACHABLE", ProbedAt: now.Add(2 * time.Second)},
		{Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy", ProbedAt: now.Add(3 * time.Second)},
	}}
	ha := haoperations.NewService(store, nil, nil, nil, nil, prober)

	degraded, err := ha.ProbeNode(ctx, nodeID)
	if err == nil || degraded.Status != "failed" {
		t.Fatalf("degraded fake DNS probe result=%#v err=%v", degraded, err)
	}
	var queued, channels, events int
	var queuedStatus string
	var nextAttempt *time.Time
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM notification_channels WHERE enabled`).Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM ha_operational_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT count(*),COALESCE(max(status),''),max(next_attempt_at) FROM notification_deliveries`).Scan(&queued, &queuedStatus, &nextAttempt); err != nil {
		t.Fatal(err)
	}
	if queued != 1 || queuedStatus != "pending" || nextAttempt == nil {
		t.Fatalf("degraded transition queue channels=%d events=%d count=%d status=%q next=%v", channels, events, queued, queuedStatus, nextAttempt)
	}
	if processed, err := notifications.DeliverNext(ctx); err != nil || !processed {
		t.Fatalf("degraded delivery processed=%v err=%v", processed, err)
	}
	if _, err := ha.ProbeNode(ctx, nodeID); err == nil {
		t.Fatal("stable degraded fake DNS probe unexpectedly succeeded")
	}
	if processed, err := notifications.DeliverNext(ctx); err != nil || processed {
		t.Fatalf("stable degraded state queued duplicate processed=%v err=%v", processed, err)
	}
	if _, err := ha.ProbeNode(ctx, nodeID); err != nil {
		t.Fatal(err)
	}
	if processed, err := notifications.DeliverNext(ctx); err != nil || !processed {
		t.Fatalf("recovery delivery processed=%v err=%v", processed, err)
	}

	result, err := notifications.Test(ctx, actor, channel.ID)
	if err != nil || !result.Success {
		t.Fatalf("test result=%#v err=%v", result, err)
	}
	receiverMu.Lock()
	payloads := append([]map[string]any(nil), received...)
	receiverMu.Unlock()
	if len(payloads) != 3 || payloads[0]["type"] != "dns.failed" || payloads[1]["type"] != "dns.recovered" || payloads[2]["type"] != "notification.test" {
		t.Fatalf("received payloads=%#v", payloads)
	}
	encoded, _ := json.Marshal(payloads)
	if strings.Contains(string(encoded), "token=hidden") || strings.Contains(string(encoded), "private") {
		t.Fatalf("destination leaked into webhook payloads: %s", encoded)
	}

	first, err := ha.History(ctx, haoperations.HistoryRequest{ClusterID: clusterID, Limit: 2})
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first history=%#v err=%v", first, err)
	}
	second, err := ha.History(ctx, haoperations.HistoryRequest{ClusterID: clusterID, Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 2 || !second.HasMore {
		t.Fatalf("second history=%#v err=%v", second, err)
	}
	third, err := ha.History(ctx, haoperations.HistoryRequest{ClusterID: clusterID, Limit: 2, Cursor: second.NextCursor})
	if err != nil || len(third.Items) != 1 || third.HasMore {
		t.Fatalf("third history=%#v err=%v", third, err)
	}
	all := append(append(first.Items, second.Items...), third.Items...)
	delivered, tests := 0, 0
	seen := map[string]bool{}
	for _, item := range all {
		if seen[item.ID] {
			t.Fatalf("duplicate history item across cursor pages: %s", item.ID)
		}
		seen[item.ID] = true
		if item.Notification != nil {
			if item.Notification.Status != "delivered" || item.Notification.AttemptCount != 1 || item.Notification.HTTPStatus == nil || *item.Notification.HTTPStatus != http.StatusNoContent {
				t.Fatalf("notification history=%#v", item)
			}
			delivered++
			if item.Notification.Test {
				tests++
			}
		}
	}
	if delivered != 3 || tests != 1 {
		t.Fatalf("history delivered=%d tests=%d items=%#v", delivered, tests, all)
	}
	historyJSON, _ := json.Marshal(all)
	if strings.Contains(string(historyJSON), "token=hidden") || strings.Contains(string(historyJSON), "/private") {
		t.Fatalf("destination leaked into history: %s", historyJSON)
	}
}

func TestRelease102MultipleWebhookDestinationsRecordMixedResults(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	const (
		userID    = "10220000-0000-4000-8000-000000000001"
		clusterID = "10220000-0000-4000-8000-000000000002"
		eventID   = "10220000-0000-4000-8000-000000000003"
	)
	if _, err := store.Pool().Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,role,enabled,created_at,updated_at)
		VALUES($1,'release-102-multiple@example.test','Administrator','hash','administrator',true,$2,$2)`, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO clusters(id,name,created_at,updated_at) VALUES($1,'Multiple webhooks',$2,$2)`, clusterID, now); err != nil {
		t.Fatal(err)
	}
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/failed" {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	cipher, err := auth.NewCredentialCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	notifications := haoperations.NewNotificationService(store, cipher, receiver.Client())
	actor := domain.Actor{UserID: userID, RequestID: "release-1.0.2-multiple"}
	if _, err := notifications.Create(ctx, actor, clusterID, "Accepted", receiver.URL+"/accepted?token=hidden", true); err != nil {
		t.Fatal(err)
	}
	if _, err := notifications.Create(ctx, actor, clusterID, "Rejected", receiver.URL+"/failed?token=hidden", true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHAEvent(ctx, haoperations.Event{ID: eventID, ClusterID: clusterID, EventType: "redundancy.degraded", Severity: "warning", Summary: "DNS redundancy degraded", Details: map[string]any{}, OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if processed, err := notifications.DeliverNext(ctx); err != nil || !processed {
			t.Fatalf("initial mixed delivery processed=%v err=%v", processed, err)
		}
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE notification_deliveries SET attempt_count=4,next_attempt_at=now() WHERE status='failed'`); err != nil {
		t.Fatal(err)
	}
	if processed, err := notifications.DeliverNext(ctx); err != nil || !processed {
		t.Fatalf("terminal mixed delivery processed=%v err=%v", processed, err)
	}
	items, err := store.ListHAHistory(ctx, haoperations.HistoryQuery{ClusterID: clusterID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, item := range items {
		if item.Notification != nil {
			statuses[item.Notification.ChannelName] = item.Notification.Status
			if item.Notification.ChannelName == "Rejected" && (item.Notification.ErrorSummary != "HTTP 503" || item.Notification.AttemptCount != 5) {
				t.Fatalf("rejected diagnostics=%#v", item.Notification)
			}
		}
	}
	if statuses["Accepted"] != "delivered" || statuses["Rejected"] != "failed" {
		t.Fatalf("mixed statuses=%#v", statuses)
	}
}
