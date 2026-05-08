package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EmbeddedApp representa un iframe registrado en el sidenav de un tenant.
//
// El campo `Origin` es la fuente de verdad para el postMessage handshake:
// el shell SOLO manda el token a este origin exacto (mitiga XSS cross-app).
type EmbeddedApp struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string  // visible en sidenav
	Origin    string  // ej. "https://app.acme.com" — sin trailing slash
	IconURL   *string // opcional
	Position  int     // orden visual; menor = más arriba
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmbeddedAppRepo: persistencia.
type EmbeddedAppRepo interface {
	// Create inserta una app nueva. Falla con ErrConflict si ya existe
	// una entrada con el mismo (tenant_id, origin).
	Create(ctx context.Context, tenantID uuid.UUID, name, origin string, iconURL *string, position int) (*EmbeddedApp, error)

	// ListByTenant devuelve las apps del tenant ordenadas por position
	// ascendente, luego por created_at.
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*EmbeddedApp, error)

	// GetByID devuelve la app si pertenece al tenant indicado.
	// Si no existe o no pertenece al tenant, devuelve ErrNotFound.
	GetByID(ctx context.Context, id, tenantID uuid.UUID) (*EmbeddedApp, error)

	// Delete elimina la app si pertenece al tenant indicado.
	Delete(ctx context.Context, id, tenantID uuid.UUID) error
}
