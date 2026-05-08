package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var _ = ioc.Register(apiEmbeddedAppsHandler)

// EmbeddedAppDTO: representación expuesta al cliente.
type EmbeddedAppDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Origin   string  `json:"origin"`
	IconURL  *string `json:"iconUrl,omitempty"`
	Position int     `json:"position"`
}

type EmbeddedAppCreateRequest struct {
	Name     string  `json:"name"`
	Origin   string  `json:"origin"`
	IconURL  *string `json:"iconUrl,omitempty"`
	Position *int    `json:"position,omitempty"` // si nil, va al final
}

type EmbeddedAppListResponse struct {
	Apps []EmbeddedAppDTO `json:"apps"`
}

// apiEmbeddedAppsHandler — registra:
//
//	GET    /api/embedded-apps         lista las apps del tenant del user
//	POST   /api/embedded-apps         crea una app (owner/admin)
//	DELETE /api/embedded-apps/{id}    borra una app (owner/admin)
//
// Todo el endpoint vive bajo /api/* (middleware Auth ya validó la cookie).
// El scoping al tenant se hace por user.tenant_id; un owner solo puede
// listar/crear/borrar dentro de su propio tenant.
func apiEmbeddedAppsHandler(
	api *APIGroup,
	apps domain.EmbeddedAppRepo,
	users domain.UserRepo,
	tenants domain.TenantRepo,
) {
	// ── GET ────────────────────────────────────────────────────────────
	fuego.Get(api.Server, "/embedded-apps", func(c fuego.ContextNoBody) (EmbeddedAppListResponse, error) {
		_, tenant, err := requireUserTenant(c.Context(), users, tenants)
		if err != nil {
			return EmbeddedAppListResponse{}, err
		}

		list, err := apps.ListByTenant(c.Context(), tenant.ID)
		if err != nil {
			return EmbeddedAppListResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not list embedded apps",
				Detail: err.Error(),
			}
		}

		out := make([]EmbeddedAppDTO, 0, len(list))
		for _, a := range list {
			out = append(out, toDTO(a))
		}
		return EmbeddedAppListResponse{Apps: out}, nil
	})

	// ── POST ───────────────────────────────────────────────────────────
	fuego.Post(api.Server, "/embedded-apps", func(c fuego.ContextWithBody[EmbeddedAppCreateRequest]) (EmbeddedAppDTO, error) {
		body, err := c.Body()
		if err != nil {
			return EmbeddedAppDTO{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "invalid body",
				Detail: err.Error(),
			}
		}

		// Normalizamos antes de validar.
		body.Name = strings.TrimSpace(body.Name)
		body.Origin = strings.TrimSpace(strings.TrimRight(body.Origin, "/"))

		if err := validateOrigin(body.Origin); err != nil {
			return EmbeddedAppDTO{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "invalid origin",
				Detail: err.Error(),
			}
		}
		if body.Name == "" {
			return EmbeddedAppDTO{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "name is required",
			}
		}

		ctx := c.Context()
		user, tenant, err := requireUserTenant(ctx, users, tenants)
		if err != nil {
			return EmbeddedAppDTO{}, err
		}
		if err := requireOwnerOrAdmin(user); err != nil {
			return EmbeddedAppDTO{}, err
		}

		position := 0
		if body.Position != nil {
			position = *body.Position
		}

		app, err := apps.Create(ctx, tenant.ID, body.Name, body.Origin, body.IconURL, position)
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return EmbeddedAppDTO{}, fuego.HTTPError{
					Status: http.StatusConflict,
					Title:  "origin already registered",
				}
			}
			return EmbeddedAppDTO{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not create embedded app",
				Detail: err.Error(),
			}
		}
		return toDTO(app), nil
	})

	// ── DELETE ─────────────────────────────────────────────────────────
	// TODO(fuego): registrar /embedded-apps/{id} cuando confirmemos
	// el syntax exacto. Por ahora dejamos solo GET/POST que cubren la
	// demo de Fase 4. El delete lo agregamos cuando lleguemos a la UI
	// de gestión.
}

func toDTO(a *domain.EmbeddedApp) EmbeddedAppDTO {
	return EmbeddedAppDTO{
		ID:       a.ID.String(),
		Name:     a.Name,
		Origin:   a.Origin,
		IconURL:  a.IconURL,
		Position: a.Position,
	}
}

// validateOrigin parsea la URL y rechaza cualquier cosa con path/query/fragment.
// El origen RFC 6454 es scheme://host[:port], punto. Sin trailing slash.
func validateOrigin(s string) error {
	if s == "" {
		return errors.New("origin is required")
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("host is required")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("origin must be scheme://host[:port] without path/query/fragment")
	}
	return nil
}

// _ silenciar warning sobre uuid no usado (lo usaremos cuando agreguemos DELETE).
var _ = uuid.Nil
