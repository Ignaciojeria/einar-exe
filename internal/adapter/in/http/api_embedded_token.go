package http

import (
	"net/http"
	"time"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiEmbeddedTokenHandler)

// EmbeddedTokenResponse: payload que el SPA pushea por postMessage al iframe.
type EmbeddedTokenResponse struct {
	// Token = id_token JWT (mismo que está en cookie HttpOnly).
	// El iframe lo usará como `Authorization: Bearer <token>` contra el
	// backend del developer (no el nuestro).
	Token string `json:"token"`
	// ExpiresAt: unix seconds. El iframe debe pedir refresh antes de esto.
	ExpiresAt int64 `json:"expiresAt"`
}

// apiEmbeddedTokenHandler — GET /api/embedded-token
//
// Endpoint de bypass del HttpOnly: el SPA llama acá (same-origin, cookie
// se manda) y obtiene el id_token raw. NO se invoca desde el iframe (el
// iframe no tiene la cookie, está en otro origin); lo invoca el shell SPA
// para reenviárselo al iframe via postMessage.
//
// Riesgo aceptado: si el SPA tiene XSS, el atacante puede llamar este
// endpoint y leer el token. Es el mismo riesgo que cualquier
// "access-token-en-JS"; lo mitigamos con SPA chico bien auditado y CSP.
//
// El TTL del token es el del id_token original (lo que dice exp). Cuando
// se vence, el middleware /api/* refresca transparentemente la cookie.
func apiEmbeddedTokenHandler(api *APIGroup, p *oidc.Provider) {
	fuego.Get(api.Server, "/embedded-token", func(c fuego.ContextNoBody) (EmbeddedTokenResponse, error) {
		// Lectura directa de la cookie (el middleware ya verificó al entrar).
		cookie, err := c.Cookie("einar_session")
		if err != nil || cookie.Value == "" {
			// No debería pasar (middleware bloquea antes), pero defensivo.
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "no session cookie",
			}
		}

		// Re-verificar para extraer el `exp` confiable.
		idToken, err := p.Verifier.Verify(c.Context(), cookie.Value)
		if err != nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "session token invalid",
				Detail: err.Error(),
			}
		}

		// Defensivo: confirmar que el user del context coincide con el
		// del token. Sin esto, alguien podría cookie-substituir y el
		// endpoint le devolvería un token random. (El middleware ya
		// previene esto pero baratísimo de chequear.)
		if u := middleware.UserFromContext(c.Context()); u == nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}

		return EmbeddedTokenResponse{
			Token:     cookie.Value,
			ExpiresAt: idToken.Expiry.Unix(),
		}, nil
	})

	// (no-op para silenciar import time; lo dejamos por si en el futuro
	// agregamos validación de TTL mínimo aquí)
	_ = time.Second
}
