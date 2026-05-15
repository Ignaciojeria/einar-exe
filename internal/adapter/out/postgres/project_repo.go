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

var _ = ioc.Register(NewProjectRepo)

type projectRepo struct {
	pool *pgxpool.Pool
}

func NewProjectRepo(pool *pgxpool.Pool) domain.ProjectRepo {
	return &projectRepo{pool: pool}
}

const projectCols = `id, tenant_id, name, slug, path, subdomain, status, created_at, updated_at`

func scanProject(row pgx.Row) (*domain.Project, error) {
	var p domain.Project
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Slug, &p.Path, &p.Subdomain, &p.Status,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return &p, err
}

func (r *projectRepo) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	name, slug, path, subdomain, status string,
) (*domain.Project, error) {
	q := `
		INSERT INTO projects (tenant_id, name, slug, path, subdomain, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + projectCols

	p, err := scanProject(r.pool.QueryRow(ctx, q, tenantID, name, slug, path, subdomain, status))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgErrCodeUniqueViolation:
				return nil, fmt.Errorf("project slug %q: %w", slug, domain.ErrConflict)
			case pgErrCodeCheckViolation:
				return nil, fmt.Errorf("project invalid arg: %w", domain.ErrInvalidArg)
			}
		}
		return nil, fmt.Errorf("insert project: %w", err)
	}

	return p, nil
}
