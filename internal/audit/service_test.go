package audit

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

const (
	clusterA = "11111111-1111-4111-8111-111111111111"
	clusterB = "22222222-2222-4222-8222-222222222222"
)

type repositoryFake struct {
	events []domain.AuditEvent
	query  Query
}

func (r *repositoryFake) ListAuditEvents(_ context.Context, query Query) ([]domain.AuditEvent, error) {
	r.query = query
	items := append([]domain.AuditEvent(nil), r.events...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	filtered := items[:0]
	for _, item := range items {
		if query.ClusterID != "" && item.ClusterID != query.ClusterID && !(query.IncludeController && item.Scope == "controller") {
			continue
		}
		if query.BeforeAt != nil && (item.CreatedAt.After(*query.BeforeAt) || (item.CreatedAt.Equal(*query.BeforeAt) && item.ID >= query.BeforeID)) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return filtered, nil
}

func (r *repositoryFake) AuditEventByID(_ context.Context, id string) (domain.AuditEvent, error) {
	for _, event := range r.events {
		if event.ID == id {
			return event, nil
		}
	}
	return domain.AuditEvent{}, domain.NewError(domain.ErrorNotFound, "audit event was not found")
}

func TestCursorPagesUseStableTimestampAndIDOrdering(t *testing.T) {
	at := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	repository := &repositoryFake{events: []domain.AuditEvent{
		auditEvent("00000000-0000-4000-8000-000000000001", at.Add(-time.Second), clusterA),
		auditEvent("00000000-0000-4000-8000-000000000003", at, clusterA),
		auditEvent("00000000-0000-4000-8000-000000000002", at, clusterA),
	}}
	service := NewService(repository)
	first, err := service.List(context.Background(), ListRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID[len(first.Items[0].ID)-1:] != "3" || first.Items[1].ID[len(first.Items[1].ID)-1:] != "2" || first.NextCursor == "" || !first.HasMore {
		t.Fatalf("first page = %+v", first)
	}

	// A concurrent newer insert cannot shift the already-issued older cursor.
	repository.events = append(repository.events, auditEvent("00000000-0000-4000-8000-000000000004", at.Add(time.Second), clusterA))
	next, err := service.List(context.Background(), ListRequest{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID[len(next.Items[0].ID)-1:] != "1" {
		t.Fatalf("next page after concurrent insert = %+v", next.Items)
	}
}

func TestListValidatesCursorLimitAndScope(t *testing.T) {
	service := NewService(&repositoryFake{})
	for _, request := range []ListRequest{
		{Limit: 0},
		{Limit: 101},
		{Limit: 50, Cursor: "not-a-cursor"},
		{Limit: 50, ClusterID: "not-an-id"},
		{Limit: 50, IncludeController: true},
	} {
		if _, err := service.List(context.Background(), request); err == nil {
			t.Fatalf("request accepted: %+v", request)
		}
	}
}

func TestClusterScopeIncludesControllerAndExcludesOtherClusters(t *testing.T) {
	repository := &repositoryFake{events: []domain.AuditEvent{
		auditEvent("00000000-0000-4000-8000-000000000001", time.Now(), clusterA),
		auditEvent("00000000-0000-4000-8000-000000000002", time.Now(), clusterB),
		{ID: "00000000-0000-4000-8000-000000000003", Scope: "controller", CreatedAt: time.Now(), Metadata: map[string]any{}},
	}}
	page, err := NewService(repository).List(context.Background(), ListRequest{
		Limit: 50, ClusterID: clusterA, IncludeController: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("scoped items = %+v", page.Items)
	}
	for _, item := range page.Items {
		if item.ClusterID == clusterB {
			t.Fatal("other-cluster event was returned")
		}
	}
	if repository.query.ClusterID != clusterA || !repository.query.IncludeController {
		t.Fatalf("repository query = %+v", repository.query)
	}
}

func TestMetadataIsPreservedAndDefensivelyRedacted(t *testing.T) {
	metadata := SafeMetadata(map[string]any{
		"enabled":                  true,
		"destinationSummary":       "hooks.example.test/…",
		"password":                 "do-not-expose",
		"apiToken":                 "do-not-expose",
		"webhookDestination":       "do-not-expose",
		"totpCode":                 "123456",
		"recoveryCode":             "aaaaa-bbbbb-ccccc-ddddd",
		"provisioningUri":          "otpauth://do-not-expose",
		"qrCodePayload":            "do-not-expose",
		"nested":                   map[string]any{"token": "do-not-expose", "errorCode": "SAFE_CODE"},
		"queryLogRetentionSeconds": 720,
	})
	if metadata["enabled"] != true || metadata["destinationSummary"] != "hooks.example.test/…" || metadata["queryLogRetentionSeconds"] != 720 {
		t.Fatalf("safe metadata was lost: %#v", metadata)
	}
	if metadata["password"] != "[REDACTED]" {
		t.Fatalf("password = %#v", metadata["password"])
	}
	if metadata["apiToken"] != "[REDACTED]" || metadata["webhookDestination"] != "[REDACTED]" {
		t.Fatalf("repository-equivalent secrets were not redacted: %#v", metadata)
	}
	for _, key := range []string{"totpCode", "recoveryCode", "provisioningUri", "qrCodePayload"} {
		if metadata[key] != "[REDACTED]" {
			t.Fatalf("MFA metadata %q was not redacted: %#v", key, metadata)
		}
	}
	nested := metadata["nested"].(map[string]any)
	if nested["token"] != "[REDACTED]" || nested["errorCode"] != "SAFE_CODE" {
		t.Fatalf("nested metadata = %#v", nested)
	}

	oversized := map[string]any{}
	for index := 0; index < 100; index++ {
		oversized[string(rune('a'+index%26))+time.Duration(index).String()] = string(make([]byte, 700))
	}
	bounded := SafeMetadata(oversized)
	if len(bounded) > maxMetadataKeys+1 || bounded["truncated"] != true {
		t.Fatalf("metadata was not bounded: keys=%d truncated=%#v", len(bounded), bounded["truncated"])
	}
}

func auditEvent(id string, at time.Time, clusterID string) domain.AuditEvent {
	return domain.AuditEvent{ID: id, CreatedAt: at, ClusterID: clusterID, Metadata: map[string]any{"safe": true}}
}
