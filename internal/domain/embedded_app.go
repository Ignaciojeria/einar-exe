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
	IsSystem  bool    // true = aprovisionada por la plataforma, no se puede borrar
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmbeddedAppPatch agrupa los campos editables. Punteros = "no tocar si nil".
type EmbeddedAppPatch struct {
	Name     *string
	IconURL  *string // *string es "setea"; *string a un "" es "setea vacío"; nil es "no tocar"
	Position *int
}

// EmbeddedAppRepo: persistencia.
type EmbeddedAppRepo interface {
	// Create inserta una app nueva (custom). Falla con ErrConflict si ya
	// existe una entrada con el mismo (tenant_id, origin).
	Create(ctx context.Context, tenantID uuid.UUID, name, origin string, iconURL *string, position int) (*EmbeddedApp, error)

	// CreateSystem inserta una app system (la plataforma la marca como
	// no-borrable). La usaremos en Fase 5/6 al provisionar OO/Redash.
	CreateSystem(ctx context.Context, tenantID uuid.UUID, name, origin string, iconURL *string, position int) (*EmbeddedApp, error)

	// ListByTenant devuelve las apps del tenant ordenadas por
	// (is_system DESC, position ASC, created_at ASC) — las system
	// arriba, las custom abajo.
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*EmbeddedApp, error)

	// GetByID devuelve la app si pertenece al tenant indicado.
	// Si no existe o no pertenece al tenant, devuelve ErrNotFound.
	GetByID(ctx context.Context, id, tenantID uuid.UUID) (*EmbeddedApp, error)

	// Update aplica un patch parcial. Devuelve ErrNotFound si no existe;
	// no permite tocar `is_system` ni `origin` (esos campos requieren
	// re-crear la app).
	Update(ctx context.Context, id, tenantID uuid.UUID, patch EmbeddedAppPatch) (*EmbeddedApp, error)

	// Delete elimina la app. Falla con ErrInvalidArg si is_system=true
	// (las system solo se borran via cleanup de tenant).
	Delete(ctx context.Context, id, tenantID uuid.UUID) error
}
