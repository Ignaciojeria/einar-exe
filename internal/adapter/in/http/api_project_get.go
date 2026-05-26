package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiProjectGetHandler)

// ProjectPublicConfig es el subset seguro de ProjectRuntimeConfig
// que se devuelve por GET. Sin secretos inline.
type ProjectPublicConfig struct {
	Version   int                       `json:"version"`
	ProjectID string                    `json:"projectId"`
	Slug      string                    `json:"slug"`
	Workspace domain.RuntimeWorkspace   `json:"workspace"`
	VM        *domain.RuntimeVM         `json:"vm,omitempty"`
	Sync      *domain.RuntimeSync       `json:"sync,omitempty"`
	Database  *PublicDatabaseConfig     `json:"database,omitempty"`
	Metadata  domain.RuntimeMetadata    `json:"metadata"`
}

// PublicDatabaseConfig = database info sin password, solo SecretRef.
type PublicDatabaseConfig struct {
	Name              string `json:"name"`
	User              string `json:"user"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	PasswordSecretRef string `json:"passwordSecretRef"`
}

func apiProjectGetHandler(
	api *APIGroup,
	users domain.UserRepo,
	tenants domain.TenantRepo,
	projects domain.ProjectRepo,
	env environment.Conf,
) {
	fuego.Get(api.Server, "/projects/by-slug/{slug}", func(c fuego.ContextNoBody) (ProjectPublicConfig, error) {
		slug := strings.TrimSpace(c.Request().PathValue("slug"))
		if slug == "" {
			return ProjectPublicConfig{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "slug is required",
			}
		}

		if !middleware.HasScope(c.Context(), "projects:read") && !middleware.HasScope(c.Context(), "*") {
			return ProjectPublicConfig{}, fuego.HTTPError{
				Status: http.StatusForbidden,
				Title:  "missing scope",
				Detail: "projects:read scope is required",
			}
		}

		_, tenant, err := requireUserTenant(c.Context(), users, tenants)
		if err != nil {
			return ProjectPublicConfig{}, err
		}

		p, err := projects.FindBySlugAndTenant(c.Context(), slug, tenant.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return ProjectPublicConfig{}, fuego.HTTPError{
					Status: http.StatusNotFound,
					Title:  "project not found",
				}
			}
			return ProjectPublicConfig{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not load project",
				Detail: err.Error(),
			}
		}

		return buildPublicConfig(p, env), nil
	})
}

// buildPublicConfig construye el subset seguro del runtime config
// a partir de un Project persistido. Sin secretos.
func buildPublicConfig(p *domain.Project, env environment.Conf) ProjectPublicConfig {
	secretsBase := fmt.Sprintf("projects/%s", p.Slug)

	rc := ProjectPublicConfig{
		Version:   1,
		ProjectID: p.ID.String(),
		Slug:      p.Slug,
		Workspace: domain.RuntimeWorkspace{
			Branch: "main",
			Mode:   "single-owner",
		},
		Metadata: domain.RuntimeMetadata{
			OwnerUserID: "", // no se expone en GET (requeriría join con users)
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		},
	}

	// VM info: reconstruir desde subdomain/path
	if p.Subdomain != "" {
		vmName := p.Slug
		httpsURL := fmt.Sprintf("https://%s", p.Subdomain)
		sshDest := normalizeMutagenDestination(p.Subdomain, env, p.Path)

		rc.VM = &domain.RuntimeVM{
			Name:              vmName,
			HTTPSURL:          httpsURL,
			SSHDestination:    sshDest,
			RemoteProjectPath: p.Path,
		}

		mutagenDest := buildMutagenDestination(env, p.Path)
		if strings.TrimSpace(mutagenDest) == "" {
			mutagenDest = sshDest
		}
		rc.Sync = &domain.RuntimeSync{
			Provider:    "mutagen",
			Destination: mutagenDest,
			SessionName: p.Slug,
			IgnoreVCS:   true,
		}
	}

	// Database: info pública + SecretRef (sin password)
	if p.DBName != "" {
		rc.Database = &PublicDatabaseConfig{
			Name:              p.DBName,
			User:              p.DBUser,
			Host:              "db",
			Port:              5432,
			PasswordSecretRef: secretsBase + "/db/password",
		}
	}

	return rc
}
