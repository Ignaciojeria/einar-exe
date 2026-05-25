package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Project representa un backend personalizado aprovisionado por tenant.
type Project struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string
	Slug      string
	Path      string
	Subdomain string
	Status    string
	DBName    string // Database aislada en el cluster Postgres
	DBUser    string // Rol Postgres dedicado al proyecto
	DBPassword string // Password del rol (plaintext por ahora)
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectRepo define persistencia para proyectos aprovisionados.
type ProjectRepo interface {
	Create(ctx context.Context, tenantID uuid.UUID, name, slug, path, subdomain, status, dbName, dbUser, dbPassword string) (*Project, error)
}
