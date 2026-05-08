package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

const embeddedAppCols = `id, tenant_id, name, origin, icon_url, position, is_system, created_at, updated_at`

func scanEmbeddedApp(row pgx.Row) (*domain.EmbeddedApp, error) {
	var a domain.EmbeddedApp
	err := row.Scan(
		&a.ID, &a.TenantID, &a.Name, &a.Origin, &a.IconURL, &a.Position, &a.IsSystem,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return &a, err
}

func (r *embeddedAppRepo) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	name, origin string,
	iconURL *string,
	position int,
) (*domain.EmbeddedApp, error) {
	return r.insert(ctx, tenantID, name, origin, iconURL, position, false)
}

func (r *embeddedAppRepo) CreateSystem(
	ctx context.Context,
	tenantID uuid.UUID,
	name, origin string,
	iconURL *string,
	position int,
) (*domain.EmbeddedApp, error) {
	return r.insert(ctx, tenantID, name, origin, iconURL, position, true)
}

func (r *embeddedAppRepo) insert(
	ctx context.Context,
	tenantID uuid.UUID,
	name, origin string,
	iconURL *string,
	position int,
	isSystem bool,
) (*domain.EmbeddedApp, error) {
	q := `
		INSERT INTO embedded_apps (tenant_id, name, origin, icon_url, position, is_system)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + embeddedAppCols
	a, err := scanEmbeddedApp(r.pool.QueryRow(ctx, q, tenantID, name, origin, iconURL, position, isSystem))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUniqueViolation {
			return nil, fmt.Errorf("origin %q already registered: %w", origin, domain.ErrConflict)
		}
		return nil, fmt.Errorf("insert embedded_app: %w", err)
	}
	return a, nil
}

func (r *embeddedAppRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*domain.EmbeddedApp, error) {
	q := `
		SELECT ` + embeddedAppCols + `
		  FROM embedded_apps
		 WHERE tenant_id = $1
		 ORDER BY is_system DESC, position ASC, created_at ASC
	`
	rows, err := r.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("select embedded_apps: %w", err)
	}
	defer rows.Close()

	var out []*domain.EmbeddedApp
	for rows.Next() {
		a, err := scanEmbeddedApp(rows)
		if err != nil {
			return nil, fmt.Errorf("scan embedded_app: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *embeddedAppRepo) GetByID(ctx context.Context, id, tenantID uuid.UUID) (*domain.EmbeddedApp, error) {
	q := `SELECT ` + embeddedAppCols + ` FROM embedded_apps WHERE id = $1 AND tenant_id = $2`
	a, err := scanEmbeddedApp(r.pool.QueryRow(ctx, q, id, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select embedded_app: %w", err)
	}
	return a, nil
}

// Update construye el SET dinámico solo con los campos no-nil del patch.
// `origin` y `is_system` quedan deliberadamente fuera: cambiar `origin`
// implicaría re-validar y reemiti el postMessage handshake; preferimos
// que el dev borre y recree.
func (r *embeddedAppRepo) Update(
	ctx context.Context, id, tenantID uuid.UUID, patch domain.EmbeddedAppPatch,
) (*domain.EmbeddedApp, error) {
	sets := []string{}
	args := []any{id, tenantID}
	idx := 3 // $1=id, $2=tenantID

	if patch.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *patch.Name)
		idx++
	}
	if patch.IconURL != nil {
		sets = append(sets, fmt.Sprintf("icon_url = $%d", idx))
		args = append(args, *patch.IconURL)
		idx++
	}
	if patch.Position != nil {
		sets = append(sets, fmt.Sprintf("position = $%d", idx))
		args = append(args, *patch.Position)
		idx++
	}
	if len(sets) == 0 {
		// Nada que actualizar: igual devolvemos el estado actual para
		// que el caller no tenga que distinguir.
		return r.GetByID(ctx, id, tenantID)
	}
	sets = append(sets, "updated_at = now()")

	q := `UPDATE embedded_apps
	         SET ` + strings.Join(sets, ", ") + `
	       WHERE id = $1 AND tenant_id = $2
	   RETURNING ` + embeddedAppCols

	a, err := scanEmbeddedApp(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update embedded_app: %w", err)
	}
	return a, nil
}

func (r *embeddedAppRepo) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	// Pre-check defensivo: si es system, error sin tocar la fila.
	app, err := r.GetByID(ctx, id, tenantID)
	if err != nil {
		return err
	}
	if app.IsSystem {
		return fmt.Errorf("cannot delete system app %q: %w", app.Name, domain.ErrInvalidArg)
	}

	q := `DELETE FROM embedded_apps WHERE id = $1 AND tenant_id = $2 AND is_system = FALSE`
	tag, err := r.pool.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete embedded_app: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
