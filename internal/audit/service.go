package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type Query struct {
	Limit             int
	BeforeAt          *time.Time
	BeforeID          string
	ClusterID         string
	IncludeController bool
}

type Repository interface {
	ListAuditEvents(context.Context, Query) ([]domain.AuditEvent, error)
	AuditEventByID(context.Context, string) (domain.AuditEvent, error)
}

type Service struct {
	repository Repository
}

type ListRequest struct {
	Limit             int
	Cursor            string
	ClusterID         string
	IncludeController bool
}

type Page struct {
	Items      []domain.AuditEvent `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
	HasMore    bool                `json:"hasMore"`
}

type cursorPayload struct {
	Version int    `json:"v"`
	At      string `json:"at"`
	ID      string `json:"id"`
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, request ListRequest) (Page, error) {
	if request.Limit < 1 || request.Limit > 100 {
		return Page{}, domain.Validation("limit", "must be between 1 and 100")
	}
	if request.ClusterID != "" && !domain.ValidID(request.ClusterID) {
		return Page{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if request.IncludeController && request.ClusterID == "" {
		return Page{}, domain.Validation("includeController", "requires clusterId")
	}
	query := Query{Limit: request.Limit + 1, ClusterID: request.ClusterID, IncludeController: request.IncludeController}
	if request.Cursor != "" {
		at, id, err := decodeCursor(request.Cursor)
		if err != nil {
			return Page{}, domain.Validation("cursor", "is invalid or unsupported")
		}
		query.BeforeAt, query.BeforeID = &at, id
	}
	items, err := s.repository.ListAuditEvents(ctx, query)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(page.Items) > request.Limit {
		page.Items = page.Items[:request.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
		page.HasMore = true
	}
	for index := range page.Items {
		page.Items[index] = present(page.Items[index])
		if request.ClusterID != "" && page.Items[index].Scope != "controller" {
			page.Items[index].Scope = "cluster"
			page.Items[index].ClusterID = request.ClusterID
		}
	}
	return page, nil
}

func (s *Service) Detail(ctx context.Context, id string) (domain.AuditEvent, error) {
	if !domain.ValidID(id) {
		return domain.AuditEvent{}, domain.Validation("auditEventId", "must be a valid UUID")
	}
	event, err := s.repository.AuditEventByID(ctx, id)
	if err != nil {
		return domain.AuditEvent{}, err
	}
	return present(event), nil
}

func present(event domain.AuditEvent) domain.AuditEvent {
	event.Metadata = SafeMetadata(event.Metadata)
	return event
}

func encodeCursor(at time.Time, id string) string {
	body, _ := json.Marshal(cursorPayload{Version: 1, At: at.UTC().Format(time.RFC3339Nano), ID: id})
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeCursor(value string) (time.Time, string, error) {
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	var payload cursorPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Version != 1 || !domain.ValidID(payload.ID) {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, payload.At)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor")
	}
	return at.UTC(), payload.ID, nil
}
