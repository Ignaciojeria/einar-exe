package postgres

import (
	"context"
	"errors"
	"fmt"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = ioc.Register(NewEmbeddedAppRepo)

type embeddedAppRepo struct {
	pool *pgxpool.Pool
}

func NewEmbeddedAppRepo(pool *pgxpool.Pool) domain.EmbeddedAppRepo {
	return &embeddedAppRepo{pool: pool}
}

func (r *embeddedAppRepo) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	name, origin string,
	iconURL *string,
	position int,
) (*domain.EmbeddedApp, error) {
	const q = `
		INSERT INTO embedded_apps (tenant_id, name, origin, icon_url, position)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, name, origin, icon_url, position, created_at, updated_at
	`
	var a domain.EmbeddedApp
	err := r.pool.QueryRow(ctx, q, tenantID, name, origin, iconURL, position).Scan(
		&a.ID, &a.TenantID, &a.Name, &a.Origin, &a.IconURL, &a.Position, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUniqueViolation {
			return nil, fmt.Errorf("origin %q already registered: %w", origin, domain.ErrConflict)
		}
		return nil, fmt.Errorf("insert embedded_app: %w", err)
	}
	return &a, nil
}

func (r *embeddedAppRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*domain.EmbeddedApp, error) {
	const q = `
		SELECT id, tenant_id, name, origin, icon_url, position, created_at, updated_at
		  FROM embedded_apps
		 WHERE tenant_id = $1
		 ORDER BY position ASC, created_at ASC
	`
	rows, err := r.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("select embedded_apps: %w", err)
	}
	defer rows.Close()

	var out []*domain.EmbeddedApp
	for rows.Next() {
		var a domain.EmbeddedApp
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Name, &a.Origin, &a.IconURL, &a.Position, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan embedded_app: %w", err)
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (r *embeddedAppRepo) GetByID(ctx context.Context, id, tenantID uuid.UUID) (*domain.EmbeddedApp, error) {
	const q = `
		SELECT id, tenant_id, name, origin, icon_url, position, created_at, updated_at
		  FROM embedded_apps
		 WHERE id = $1 AND tenant_id = $2
	`
	var a domain.EmbeddedApp
	err := r.pool.QueryRow(ctx, q, id, tenantID).Scan(
		&a.ID, &a.TenantID, &a.Name, &a.Origin, &a.IconURL, &a.Position, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select embedded_app: %w", err)
	}
	return &a, nil
}

func (r *embeddedAppRepo) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	const q = `DELETE FROM embedded_apps WHERE id = $1 AND tenant_id = $2`
	tag, err := r.pool.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete embedded_app: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
