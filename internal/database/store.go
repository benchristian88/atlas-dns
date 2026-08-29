package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auditservice "github.com/benchristian88/atlas-dns/internal/audit"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Pool() *pgxpool.Pool            { return s.pool }
func (s *Store) Close()                         { s.pool.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func audit(ctx context.Context, tx pgx.Tx, event domain.AuditEvent) error {
	metadata, err := json.Marshal(auditservice.SafeMetadata(event.Metadata))
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events
			(id, actor_type, actor_user_id, action, resource_type, resource_id, request_id, metadata_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.ID, event.ActorType, event.ActorUserID, event.Action, event.ResourceType,
		event.ResourceID, event.RequestID, metadata, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func mapDatabaseError(err error, resource string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewError(domain.ErrorNotFound, resource+" was not found")
	}
	return err
}
