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

// Lista de columnas reusable. Mantener sincronizada con el orden
// del Scan en scanTenant.
const tenantCols = `id, slug, display_name,
	casdoor_org,
	openobserve_org_id, openobserve_user_email, openobserve_user_password,
	metabase_group_id, metabase_collection_id,
	metabase_user_email, metabase_user_password,
	created_at, updated_at`

func (r *tenantRepo) Create(ctx context.Context, slug, displayName string) (*domain.Tenant, error) {
	q := `
		INSERT INTO tenants (slug, display_name)
		VALUES ($1, $2)
		RETURNING ` + tenantCols
	t, err := scanTenant(r.pool.QueryRow(ctx, q, slug, displayName))
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
	return t, nil
}

// scanTenant centraliza el Scan en el mismo orden que `tenantCols`.
func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	var t domain.Tenant
	err := row.Scan(
		&t.ID, &t.Slug, &t.DisplayName,
		&t.CasdoorOrg,
		&t.OpenObserveOrgID, &t.OpenObserveUserEmail, &t.OpenObserveUserPassword,
		&t.MetabaseGroupID, &t.MetabaseCollectionID,
		&t.MetabaseUserEmail, &t.MetabaseUserPassword,
		&t.CreatedAt, &t.UpdatedAt,
	)
	return &t, err
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
	return r.setOrgRef(ctx, id, "casdoor_org", orgName)
}

func (r *tenantRepo) SetOpenObserveOrgID(ctx context.Context, id uuid.UUID, orgID string) error {
	return r.setOrgRef(ctx, id, "openobserve_org_id", orgID)
}

func (r *tenantRepo) SetOpenObserveCredentials(ctx context.Context, id uuid.UUID, email, password string) error {
	q := `UPDATE tenants
	         SET openobserve_user_email    = $2,
	             openobserve_user_password = $3,
	             updated_at                = now()
	       WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, email, password)
	if err != nil {
		return fmt.Errorf("update tenant openobserve credentials: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *tenantRepo) SetMetabaseProvisioning(
	ctx context.Context, id uuid.UUID,
	groupID, collectionID int, email, password string,
) error {
	q := `UPDATE tenants
	         SET metabase_group_id      = $2,
	             metabase_collection_id = $3,
	             metabase_user_email    = $4,
	             metabase_user_password = $5,
	             updated_at             = now()
	       WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, groupID, collectionID, email, password)
	if err != nil {
		return fmt.Errorf("update tenant metabase provisioning: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// setOrgRef centraliza UPDATE de cualquier columna `*_org*` que linkee
// al tenant con un sistema externo. La columna se inyecta hardcoded
// (whitelist arriba), no viene del cliente, no hay riesgo de SQLi.
func (r *tenantRepo) setOrgRef(ctx context.Context, id uuid.UUID, col, val string) error {
	q := fmt.Sprintf(`UPDATE tenants SET %s = $2, updated_at = now() WHERE id = $1`, col)
	tag, err := r.pool.Exec(ctx, q, id, val)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUniqueViolation {
			return fmt.Errorf("%s %q already linked: %w", col, val, domain.ErrConflict)
		}
		return fmt.Errorf("update tenant.%s: %w", col, err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *tenantRepo) FindBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return r.findOne(ctx, `SELECT `+tenantCols+` FROM tenants WHERE slug = $1`, slug)
}

func (r *tenantRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return r.findOne(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id)
}

func (r *tenantRepo) findOne(ctx context.Context, q string, arg any) (*domain.Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx, q, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select tenant: %w", err)
	}
	return t, nil
}
