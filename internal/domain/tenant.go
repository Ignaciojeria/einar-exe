// Package domain define las entidades de negocio y las interfaces de
// repositorios. La capa de adapter (postgres) las implementa; los
// handlers HTTP solo dependen de estas interfaces.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio (sentinels). Las capas de arriba los chequean con
// errors.Is(...) sin acoplarse a SQL/pgx.
var (
	ErrNotFound   = errors.New("entity not found")
	ErrConflict   = errors.New("entity already exists")
	ErrInvalidArg = errors.New("invalid argument")
)

// Tenant = workspace en la plataforma. Aísla users y embedded apps.
type Tenant struct {
	ID                      uuid.UUID
	Slug                    string  // /t/{slug}/...
	DisplayName             string  // nombre humano-friendly
	CasdoorOrg              *string // nullable hasta provisioning
	OpenObserveOrgID        *string // identifier random asignado por OO; nullable
	OpenObserveUserEmail    *string // user dedicado por tenant en OO
	OpenObserveUserPassword *string // sensible; ver TODO de encriptación at-rest
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// Role dentro de un tenant.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User = mapping local del JWT subject a su tenant + rol.
// El perfil completo (nombre, foto, email verificado) lo lee la app
// del JWT, no de aquí.
type User struct {
	ID         uuid.UUID
	TenantID   *uuid.UUID // NULL hasta que completa /signup
	CasdoorSub string     // claim `sub` del JWT (identificador estable)
	Email      *string
	Role       Role
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// HasTenant es shortcut para los handlers que deciden si redirigir
// a /signup.
func (u *User) HasTenant() bool { return u.TenantID != nil }

// TenantRepo: persistencia de tenants.
type TenantRepo interface {
	// Create inserta un tenant nuevo. Si el slug ya existe,
	// devuelve ErrConflict.
	Create(ctx context.Context, slug, displayName string) (*Tenant, error)

	// Delete borra un tenant por ID. Compensación para flujos de
	// signup que fallan a mitad de camino.
	Delete(ctx context.Context, id uuid.UUID) error

	// SetCasdoorOrg marca el tenant como aprovisionado en Casdoor.
	SetCasdoorOrg(ctx context.Context, id uuid.UUID, orgName string) error

	// SetOpenObserveOrgID guarda el identifier que devolvió OpenObserve
	// al crear la org. Lo necesitamos para construir URLs de iframe.
	SetOpenObserveOrgID(ctx context.Context, id uuid.UUID, orgID string) error

	// SetOpenObserveCredentials guarda el user/pwd dedicado del tenant
	// en OO. Se llama una vez al signup y al rotar.
	SetOpenObserveCredentials(ctx context.Context, id uuid.UUID, email, password string) error

	// FindBySlug devuelve el tenant o ErrNotFound.
	FindBySlug(ctx context.Context, slug string) (*Tenant, error)

	// FindByID devuelve el tenant o ErrNotFound.
	FindByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
}

// UserRepo: persistencia de users.
type UserRepo interface {
	// EnsureBySub crea o devuelve el user identificado por el sub
	// del JWT. Idempotente. Email se actualiza si cambió.
	// El tenant_id NO se asigna aquí (eso lo hace AssignTenant).
	EnsureBySub(ctx context.Context, sub string, email *string) (*User, error)

	// FindBySub devuelve el user o ErrNotFound.
	FindBySub(ctx context.Context, sub string) (*User, error)

	// AssignTenant asocia un user con un tenant y un rol.
	// Falla con ErrConflict si el user ya tiene tenant distinto.
	AssignTenant(ctx context.Context, userID, tenantID uuid.UUID, role Role) error
}
