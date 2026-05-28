package postgres

import (
	"context"
	"errors"
	"fmt"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = ioc.Register(NewUserRepo)

type userRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) domain.UserRepo {
	return &userRepo{pool: pool}
}

// EnsureByExeDevID: upsert por exedev_user_id (UNIQUE). El email se
// refresca cada vez (puede haber cambiado en exe.dev).
func (r *userRepo) EnsureByExeDevID(ctx context.Context, exedevUserID string, email *string) (*domain.User, error) {
	const q = `
		INSERT INTO users (exedev_user_id, email)
		VALUES ($1, $2)
		ON CONFLICT (exedev_user_id) DO UPDATE
		    SET email      = COALESCE(EXCLUDED.email, users.email),
		        updated_at = now()
		RETURNING id, tenant_id, exedev_user_id, email, role, created_at, updated_at
	`
	return r.scanOne(ctx, q, exedevUserID, email)
}

func (r *userRepo) FindByExeDevID(ctx context.Context, exedevUserID string) (*domain.User, error) {
	const q = `
		SELECT id, tenant_id, exedev_user_id, email, role, created_at, updated_at
		FROM users WHERE exedev_user_id = $1
	`
	u, err := r.scanOne(ctx, q, exedevUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return u, err
}

func (r *userRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const q = `
		SELECT id, tenant_id, exedev_user_id, email, role, created_at, updated_at
		FROM users WHERE id = $1
	`
	u, err := r.scanOne(ctx, q, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return u, err
}

// AssignTenant: setea tenant_id + role. Si el user ya pertenece a otro
// tenant (tenant_id IS NOT NULL y distinto), devuelve ErrConflict.
func (r *userRepo) AssignTenant(ctx context.Context, userID, tenantID uuid.UUID, role domain.Role) error {
	const q = `
		UPDATE users
		   SET tenant_id  = $2,
		       role       = $3,
		       updated_at = now()
		 WHERE id = $1
		   AND (tenant_id IS NULL OR tenant_id = $2)
	`
	tag, err := r.pool.Exec(ctx, q, userID, tenantID, string(role))
	if err != nil {
		return fmt.Errorf("update users.tenant_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var existingTenant *uuid.UUID
		err := r.pool.QueryRow(ctx,
			`SELECT tenant_id FROM users WHERE id = $1`, userID,
		).Scan(&existingTenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("check existing tenant: %w", err)
		}
		return fmt.Errorf("user already belongs to tenant %s: %w",
			existingTenant, domain.ErrConflict)
	}
	return nil
}

func (r *userRepo) scanOne(ctx context.Context, q string, args ...any) (*domain.User, error) {
	var u domain.User
	var role string
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&u.ID, &u.TenantID, &u.ExeDevUserID, &u.Email, &role, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.Role = domain.Role(role)
	return &u, nil
}
