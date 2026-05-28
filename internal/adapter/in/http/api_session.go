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
//
// Decisión: NO exponemos el id_token aquí. El shell SPA lo necesitará
// solo cuando haya que pasarlo a iframes embebidos (Fase 4); en ese
// momento crearemos un endpoint dedicado `/api/embedded-token` que solo
// devuelve el token a peticiones same-origin con header explícito.
type SessionResponse struct {
	User          MeResponse `json:"user"`
	Authenticated bool       `json:"authenticated"`
	// TenantSlug = workspace al que pertenece el user. Vacío si todavía
	// no completó /signup; el frontend usa eso para redirigir.
	TenantSlug string `json:"tenantSlug,omitempty"`
}

// apiSessionHandler — GET /api/session
//
// Endpoint principal del shell para saber "¿quién soy?" al arrancar.
// Si la sesión está viva (incluso tras refresh transparente) devuelve
// authenticated=true; si no, el middleware ya respondió 401 antes y este
// handler nunca corre.
//
// También resuelve el tenant del user (si existe) y lo expone como
// `tenantSlug` para que el frontend sepa a qué workspace dirigirse.
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
			User:          claimsToMe(u),
			Authenticated: true,
		}

		// Resolver tenant. Si el user todavía no se persistió (raro: solo
		// pasaría en race entre /auth/callback y /api/session), tratamos
		// como "sin tenant" sin romper la sesión.
		dbUser, err := users.FindBySub(c.Context(), u.Sub)
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
