package http

import (
	"net/http"

	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiSessionHandler)

// SessionResponse: payload que el shell SPA consume al cargar.
//
// Decisi\u00f3n: NO exponemos el id_token aqu\u00ed. El shell SPA lo necesitar\u00e1
// solo cuando haya que pasarlo a iframes embebidos (Fase 4); en ese
// momento crearemos un endpoint dedicado `/api/embedded-token` que solo
// devuelve el token a peticiones same-origin con header expl\u00edcito.
//
// `/api/session` se queda con metadatos: qui\u00e9n es el user, est\u00e1 vivo,
// y \u2014cuando exista\u2014 a qu\u00e9 tenant pertenece.
type SessionResponse struct {
	User          MeResponse `json:"user"`
	Authenticated bool       `json:"authenticated"`
	// TenantSlug se llenar\u00e1 en Fase 1 cuando exista la tabla `tenants`.
	TenantSlug string `json:"tenantSlug,omitempty"`
}

// apiSessionHandler \u2014 GET /api/session
//
// Endpoint principal del shell para saber "\u00bfqui\u00e9n soy?" al arrancar.
// Si la sesi\u00f3n est\u00e1 viva (incluso tras refresh transparente) devuelve
// authenticated=true; si no, el middleware ya respondi\u00f3 401 antes y este
// handler nunca corre.
func apiSessionHandler(api *APIGroup) {
	fuego.Get(api.Server, "/session", func(c fuego.ContextNoBody) (SessionResponse, error) {
		u := middleware.UserFromContext(c.Context())
		c.SetHeader("Content-Type", "application/json")
		if u == nil {
			return SessionResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}
		return SessionResponse{
			User:          claimsToMe(u),
			Authenticated: true,
		}, nil
	})
}
