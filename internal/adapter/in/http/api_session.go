package http

import (
	"errors"
	"net/http"

	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiSessionHandler)

// SessionResponse: payload que el shell SPA consume al cargar.
type SessionResponse struct {
	User          MeResponse `json:"user"`
	Authenticated bool       `json:"authenticated"`
	TenantSlug    string     `json:"tenantSlug,omitempty"`
}

// apiSessionHandler — GET /api/session
func apiSessionHandler(api *APIGroup, users domain.UserRepo, tenants domain.TenantRepo) {
	fuego.Get(api.Server, "/session", func(c fuego.ContextNoBody) (SessionResponse, error) {
		u := middleware.UserFromContext(c.Context())
		if u == nil {
			return SessionResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}

		resp := SessionResponse{
			User: MeResponse{
				ExeDevUserID: u.ExeDevUserID,
				Email:        u.Email,
			},
			Authenticated: true,
		}

		dbUser, err := users.FindByExeDevID(c.Context(), u.ExeDevUserID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return resp, nil
		case err != nil:
			return SessionResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not load user",
				Detail: err.Error(),
			}
		}
		if !dbUser.HasTenant() {
			return resp, nil
		}

		tenant, err := tenants.FindByID(c.Context(), *dbUser.TenantID)
		if err != nil {
			return SessionResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not load tenant",
				Detail: err.Error(),
			}
		}
		resp.TenantSlug = tenant.Slug
		return resp, nil
	})
}
