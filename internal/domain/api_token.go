package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// APIToken representa un PAT emitido por un usuario.
type APIToken struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	TokenPrefix string
	TokenHash   string
	Scopes      []string
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// APITokenRepo persistencia de PATs.
type APITokenRepo interface {
	Create(ctx context.Context, userID uuid.UUID, name, tokenPrefix, tokenHash string, scopes []string, expiresAt *time.Time) (*APIToken, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*APIToken, error)
	Revoke(ctx context.Context, id, userID uuid.UUID) error
	FindActiveByHash(ctx context.Context, tokenHash string) (*APIToken, error)
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
}
