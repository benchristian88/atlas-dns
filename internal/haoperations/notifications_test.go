package haoperations

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type notificationRepositoryFake struct {
	record    NotificationChannelRecord
	delivery  NotificationDelivery
	finished  NotificationDelivery
	audits    []domain.AuditEvent
	lastAudit domain.AuditEvent
	testEvent Event
}

func (r *notificationRepositoryFake) ClusterByID(context.Context, string) (domain.Cluster, error) {
	return domain.Cluster{ID: "11111111-1111-4111-8111-111111111111"}, nil
}
func (r *notificationRepositoryFake) ListNotificationChannels(context.Context, string) ([]NotificationChannel, error) {
	return []NotificationChannel{r.record.Channel}, nil
}
func (r *notificationRepositoryFake) NotificationChannelRecord(context.Context, string) (NotificationChannelRecord, error) {
	return r.record, nil
}
func (r *notificationRepositoryFake) SaveNotificationChannel(_ context.Context, value NotificationChannelRecord, _ int, event domain.AuditEvent) error {
	r.record = value
	r.lastAudit = event
	return nil
}
func (r *notificationRepositoryFake) DeleteNotificationChannel(_ context.Context, _ string, _ int, event domain.AuditEvent) error {
	r.lastAudit = event
	return nil
}
func (r *notificationRepositoryFake) RecordAuditEvent(_ context.Context, event domain.AuditEvent) error {
	r.audits = append(r.audits, event)
	return nil
}
func (r *notificationRepositoryFake) RecordNotificationTest(_ context.Context, event Event, value NotificationDelivery, audit domain.AuditEvent) error {
	r.testEvent = event
	r.finished = value
	r.audits = append(r.audits, audit)
	return nil
}
func (r *notificationRepositoryFake) ClaimNotificationDelivery(context.Context, time.Time) (NotificationDelivery, NotificationChannelRecord, error) {
	return r.delivery, r.record, nil
}
func (r *notificationRepositoryFake) FinishNotificationDelivery(_ context.Context, value NotificationDelivery) error {
	r.finished = value
	return nil
}

type payloadProtectorFake struct{ plaintext []byte }

func (p *payloadProtectorFake) EncryptPayload(_ string, value []byte) (domain.EncryptedPayload, error) {
	p.plaintext = append([]byte(nil), value...)
	return domain.EncryptedPayload{Ciphertext: []byte("ciphertext"), Nonce: []byte("nonce"), KeyVersion: 1, Algorithm: "AES-256-GCM"}, nil
}
func (p *payloadProtectorFake) DecryptPayload(string, domain.EncryptedPayload) ([]byte, error) {
	return append([]byte(nil), p.plaintext...), nil
}

func TestNotificationDestinationIsHTTPSWriteOnly(t *testing.T) {
	repository, protector := &notificationRepositoryFake{}, &payloadProtectorFake{}
	service := NewNotificationService(repository, protector)
	actor := domain.Actor{UserID: "22222222-2222-4222-8222-222222222222", RequestID: "request"}
	if _, err := service.Create(context.Background(), actor, "11111111-1111-4111-8111-111111111111", "Unsafe", "http://receiver.test/hook", true); err == nil {
		t.Fatal("plaintext webhook accepted")
	}
	channel, err := service.Create(context.Background(), actor, "11111111-1111-4111-8111-111111111111", "Operations", "https://receiver.test/hook?token=secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if !channel.DestinationSet || channel.DestinationSummary != "https://receiver.test" || strings.Contains(string(repository.record.Destination.Ciphertext), "receiver") || strings.Contains(channel.DestinationSummary, "secret") {
		t.Fatalf("destination leaked in channel=%#v envelope=%q", channel, repository.record.Destination.Ciphertext)
	}
}

func TestNotificationListDerivesSafeSummaryForExistingEncryptedChannel(t *testing.T) {
	repository := &notificationRepositoryFake{record: NotificationChannelRecord{Channel: NotificationChannel{
		ID: "11111111-1111-4111-8111-111111111111", ClusterID: "22222222-2222-4222-8222-222222222222", Name: "Existing", DestinationSet: true,
	}}}
	protector := &payloadProtectorFake{plaintext: []byte("https://legacy.example.test/private/hook?token=hidden")}
	channels, err := NewNotificationService(repository, protector).List(context.Background(), repository.record.Channel.ClusterID)
	if err != nil || len(channels) != 1 {
		t.Fatalf("channels=%#v err=%v", channels, err)
	}
	if channels[0].DestinationSummary != "https://legacy.example.test" || strings.Contains(channels[0].DestinationSummary, "hidden") {
		t.Fatalf("unsafe legacy summary: %#v", channels[0])
	}
}

func TestNotificationUpdatePreservesOrDeliberatelyReplacesHiddenDestination(t *testing.T) {
	repository := &notificationRepositoryFake{}
	protector := &payloadProtectorFake{}
	service := NewNotificationService(repository, protector)
	actor := domain.Actor{UserID: "22222222-2222-4222-8222-222222222222", RequestID: "request"}
	created, err := service.Create(context.Background(), actor, "11111111-1111-4111-8111-111111111111", "Operations", "https://receiver.test/original?token=secret", true)
	if err != nil {
		t.Fatal(err)
	}
	originalCiphertext := string(repository.record.Destination.Ciphertext)
	originalPlaintext := string(protector.plaintext)
	updated, err := service.Update(context.Background(), actor, created.ID, "Operations renamed", nil, false, created.RecordVersion)
	if err != nil {
		t.Fatal(err)
	}
	if string(repository.record.Destination.Ciphertext) != originalCiphertext || string(protector.plaintext) != originalPlaintext || updated.Enabled {
		t.Fatalf("hidden destination was not preserved: channel=%#v", updated)
	}
	replacement := "https://replacement.test/hook?token=new-secret"
	replaced, err := service.Update(context.Background(), actor, created.ID, updated.Name, &replacement, true, updated.RecordVersion)
	if err != nil {
		t.Fatal(err)
	}
	if string(protector.plaintext) != replacement || replaced.DestinationSummary != "https://replacement.test" {
		t.Fatalf("destination was not deliberately replaced: channel=%#v plaintext=%q", replaced, protector.plaintext)
	}
}

func TestNotificationEnableDisableAndDeleteAreExplicitAndAudited(t *testing.T) {
	repository := &notificationRepositoryFake{}
	service := NewNotificationService(repository, &payloadProtectorFake{})
	actor := domain.Actor{UserID: "22222222-2222-4222-8222-222222222222", RequestID: "request"}
	channel, err := service.Create(context.Background(), actor, "11111111-1111-4111-8111-111111111111", "Operations", "https://receiver.test/hook", true)
	if err != nil {
		t.Fatal(err)
	}
	channel, err = service.Update(context.Background(), actor, channel.ID, channel.Name, nil, false, channel.RecordVersion)
	if err != nil || repository.lastAudit.Action != "notification.channel_disabled" {
		t.Fatalf("disable channel=%#v audit=%#v err=%v", channel, repository.lastAudit, err)
	}
	channel, err = service.Update(context.Background(), actor, channel.ID, channel.Name, nil, true, channel.RecordVersion)
	if err != nil || repository.lastAudit.Action != "notification.channel_enabled" {
		t.Fatalf("enable channel=%#v audit=%#v err=%v", channel, repository.lastAudit, err)
	}
	if err := service.Delete(context.Background(), actor, channel.ID, "wrong", channel.RecordVersion); err == nil {
		t.Fatal("delete accepted the wrong channel name")
	}
	if err := service.Delete(context.Background(), actor, channel.ID, channel.Name, channel.RecordVersion); err != nil || repository.lastAudit.Action != "notification.channel_removed" {
		t.Fatalf("delete audit=%#v err=%v", repository.lastAudit, err)
	}
}

func TestNotificationTestIsBoundedRedactedAndAudited(t *testing.T) {
	repository := &notificationRepositoryFake{record: NotificationChannelRecord{Channel: NotificationChannel{ID: "11111111-1111-4111-8111-111111111111", ClusterID: "22222222-2222-4222-8222-222222222222", Name: "Operations", Enabled: true, RecordVersion: 1}}}
	protector := &payloadProtectorFake{plaintext: []byte("https://receiver.test/hook?token=secret")}
	service := NewNotificationService(repository, protector)
	var sent string
	service.client = &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		sent = string(body)
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("ignored")), Header: http.Header{}}, nil
	})}
	result, err := service.Test(context.Background(), domain.Actor{UserID: "33333333-3333-4333-8333-333333333333", RequestID: "request"}, repository.record.Channel.ID)
	if err != nil || !result.Success || len(repository.audits) != 2 {
		t.Fatalf("result=%#v audits=%d err=%v", result, len(repository.audits), err)
	}
	if repository.finished.Status != "succeeded" || repository.finished.AttemptCount != 1 || repository.testEvent.EventType != "notification.test" || repository.finished.HTTPStatus == nil || *repository.finished.HTTPStatus != http.StatusNoContent {
		t.Fatalf("test history event=%#v delivery=%#v", repository.testEvent, repository.finished)
	}
	if strings.Contains(sent, "receiver.test") || strings.Contains(sent, "secret") {
		t.Fatalf("test payload leaked destination: %s", sent)
	}
}

func TestNotificationTestFailureRecordsSafeTerminalHistory(t *testing.T) {
	repository := &notificationRepositoryFake{record: NotificationChannelRecord{Channel: NotificationChannel{ID: "11111111-1111-4111-8111-111111111111", ClusterID: "22222222-2222-4222-8222-222222222222", Name: "Operations", Enabled: true, RecordVersion: 1}}}
	protector := &payloadProtectorFake{plaintext: []byte("https://receiver.test/private?token=secret")}
	service := NewNotificationService(repository, protector)
	service.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("secret remote response")), Header: http.Header{}}, nil
	})}
	result, err := service.Test(context.Background(), domain.Actor{UserID: "33333333-3333-4333-8333-333333333333", RequestID: "request"}, repository.record.Channel.ID)
	if err != nil || result.Success || result.ErrorCode != "NOTIFICATION_HTTP_REJECTED" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if repository.finished.Status != "failed" || repository.finished.ErrorSummary != "HTTP 500" || repository.finished.HTTPStatus == nil || *repository.finished.HTTPStatus != 500 {
		t.Fatalf("failed test history=%#v", repository.finished)
	}
	if strings.Contains(repository.finished.ErrorSummary, "secret") {
		t.Fatalf("remote body leaked: %#v", repository.finished)
	}
}

func TestNotificationPayloadExcludesDetailsAndDestination(t *testing.T) {
	repository := &notificationRepositoryFake{delivery: NotificationDelivery{ID: "delivery", AttemptCount: 1, Event: Event{ID: "event", ClusterID: "cluster", EventType: "dns.failed", Severity: "critical", Summary: "DNS failed", Details: map[string]any{"rawError": "secret"}, OccurredAt: time.Now()}}}
	protector := &payloadProtectorFake{plaintext: []byte("https://receiver.test/hook")}
	repository.record = NotificationChannelRecord{Channel: NotificationChannel{ID: "channel", Enabled: true}, Destination: domain.EncryptedPayload{Ciphertext: []byte("ciphertext")}}
	service := NewNotificationService(repository, protector)
	var sent string
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		sent = string(body)
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})}
	delivered, err := service.DeliverNext(context.Background())
	if err != nil || !delivered || repository.finished.Status != "succeeded" {
		t.Fatalf("delivered=%v finished=%#v err=%v", delivered, repository.finished, err)
	}
	if strings.Contains(sent, "rawError") || strings.Contains(sent, "secret") || strings.Contains(sent, "receiver.test") {
		t.Fatalf("sensitive notification payload: %s", sent)
	}
}

func TestNotificationFailureRetainsOneRetryingResultWithSafeReason(t *testing.T) {
	repository := &notificationRepositoryFake{delivery: NotificationDelivery{ID: "delivery", AttemptCount: 2, Event: Event{ID: "event", ClusterID: "cluster", EventType: "dns.failed", Severity: "critical", Summary: "DNS failed", OccurredAt: time.Now()}}}
	protector := &payloadProtectorFake{plaintext: []byte("https://receiver.test/hook")}
	repository.record = NotificationChannelRecord{Channel: NotificationChannel{ID: "channel", Enabled: true}}
	service := NewNotificationService(repository, protector)
	service.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("token=secret")), Header: http.Header{}}, nil
	})}
	processed, err := service.DeliverNext(context.Background())
	if err != nil || !processed || repository.finished.Status != "failed" || repository.finished.CompletedAt != nil || repository.finished.NextAttemptAt == nil {
		t.Fatalf("processed=%v delivery=%#v err=%v", processed, repository.finished, err)
	}
	if repository.finished.AttemptCount != 2 || repository.finished.ErrorCode != "NOTIFICATION_HTTP_REJECTED" || repository.finished.ErrorSummary != "HTTP 401" {
		t.Fatalf("retry diagnostics=%#v", repository.finished)
	}
}
