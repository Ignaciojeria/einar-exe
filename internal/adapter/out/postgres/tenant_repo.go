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

var _ = ioc.Register(NewTenantRepo)

type tenantRepo struct {
	pool *pgxpool.Pool
}

// NewTenantRepo se registra contra la INTERFACE domain.TenantRepo, no
// contra el struct concreto. Los handlers piden domain.TenantRepo y
// IoC les da este impl.
func NewTenantRepo(pool *pgxpool.Pool) domain.TenantRepo {
	return &tenantRepo{pool: pool}
}

const (
	// pgErrCodeUniqueViolation es el SQLSTATE 23505 — violación de UNIQUE.
	// Lo usamos para mapear a domain.ErrConflict sin acoplar la capa de
	// dominio a pgx.
	pgErrCodeUniqueViolation = "23505"
	// 23514 = check_violation (ej. slug que no matchea el regex).
	pgErrCodeCheckViolation = "23514"
)

func (r *tenantRepo) Create(ctx context.Context, slug, displayName string) (*domain.Tenant, error) {
	const q = `
		INSERT INTO tenants (slug, display_name)
		VALUES ($1, $2)
		RETURNING id, slug, display_name, casdoor_org, created_at, updated_at
	`
	var t domain.Tenant
	err := r.pool.QueryRow(ctx, q, slug, displayName).Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.CasdoorOrg, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgErrCodeUniqueViolation:
				return nil, fmt.Errorf("tenant slug %q: %w", slug, domain.ErrConflict)
			case pgErrCodeCheckViolation:
				return nil, fmt.Errorf("tenant slug %q invalid: %w", slug, domain.ErrInvalidArg)
			}
		}
		return nil, fmt.Errorf("insert tenant: %w", err)
	}
	return &t, nil
}

func (r *tenantRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *tenantRepo) SetCasdoorOrg(ctx context.Context, id uuid.UUID, orgName string) error {
	const q = `UPDATE tenants
	              SET casdoor_org = $2, updated_at = now()
	            WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, orgName)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUniqueViolation {
			return fmt.Errorf("casdoor_org %q already linked: %w", orgName, domain.ErrConflict)
		}
		return fmt.Errorf("update tenant.casdoor_org: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *tenantRepo) FindBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.findOne(ctx,
		`SELECT id, slug, display_name, casdoor_org, created_at, updated_at
		   FROM tenants WHERE slug = $1`, slug)
}

func (r *tenantRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return r.findOne(ctx,
		`SELECT id, slug, display_name, casdoor_org, created_at, updated_at
		   FROM tenants WHERE id = $1`, id)
}

func (r *tenantRepo) findOne(ctx context.Context, q string, arg any) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.pool.QueryRow(ctx, q, arg).Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.CasdoorOrg, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select tenant: %w", err)
	}
	return &t, nil
}
