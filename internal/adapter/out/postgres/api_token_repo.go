package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = ioc.Register(NewAPITokenRepo)

type apiTokenRepo struct{ pool *pgxpool.Pool }

func NewAPITokenRepo(pool *pgxpool.Pool) domain.APITokenRepo { return &apiTokenRepo{pool: pool} }

const apiTokenCols = `id, user_id, name, token_prefix, token_hash, scopes, last_used_at, expires_at, revoked_at, created_at, updated_at`

func scanAPIToken(row pgx.Row) (*domain.APIToken, error) {
	var t domain.APIToken
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenPrefix, &t.TokenHash, &t.Scopes, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt)
	return &t, err
}

func (r *apiTokenRepo) Create(ctx context.Context, userID uuid.UUID, name, tokenPrefix, tokenHash string, scopes []string, expiresAt *time.Time) (*domain.APIToken, error) {
	q := `INSERT INTO api_tokens (user_id, name, token_prefix, token_hash, scopes, expires_at)
	      VALUES ($1,$2,$3,$4,$5,$6)
	      RETURNING ` + apiTokenCols
	return scanAPIToken(r.pool.QueryRow(ctx, q, userID, name, tokenPrefix, tokenHash, scopes, expiresAt))
}

func (r *apiTokenRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.APIToken, error) {
	q := `SELECT ` + apiTokenCols + ` FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("select api_tokens: %w", err)
	}
	defer rows.Close()
	out := []*domain.APIToken{}
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *apiTokenRepo) Revoke(ctx context.Context, id, userID uuid.UUID) error {
	q := `UPDATE api_tokens SET revoked_at = now(), updated_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("revoke api_token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *apiTokenRepo) FindActiveByHash(ctx context.Context, tokenHash string) (*domain.APIToken, error) {
	q := `SELECT ` + apiTokenCols + `
	      FROM api_tokens
	      WHERE token_hash = $1
	        AND revoked_at IS NULL
	        AND (expires_at IS NULL OR expires_at > now())`
	t, err := scanAPIToken(r.pool.QueryRow(ctx, q, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select active api_token: %w", err)
	}
	return t, nil
}

func (r *apiTokenRepo) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now(), updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("touch api_token: %w", err)
	}
	return nil
}
