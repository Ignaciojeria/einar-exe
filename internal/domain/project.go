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
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectRepo define persistencia para proyectos aprovisionados.
type ProjectRepo interface {
	Create(ctx context.Context, tenantID uuid.UUID, name, slug, path, subdomain, status string) (*Project, error)
}
