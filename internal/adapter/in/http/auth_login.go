package http

import (
	"net/http"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"golang.org/x/oauth2"
)

var _ = ioc.Register(authLoginHandler)

// authLoginHandler inicia el flujo OAuth/OIDC.
//
// Flujo:
//  1. Genera un `state` aleatorio (CSRF) y lo guarda en cookie efímera.
//  2. Opcionalmente guarda `?return=/ruta` para volver tras el login.
//  3. Construye la authorize URL con `client_id`, `redirect_uri`, `scope`,
//     `state` y `provider=provider_google_einar` para que Casdoor salte
//     directo a Google sin renderizar su UI.
//  4. 302 al browser hacia la authorize URL.
func authLoginHandler(s *fuego.Server, p *oidc.Provider, env environment.Conf) {
	fuego.Get(s, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		state, err := newStateToken()
		if err != nil {
			return nil, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not generate state",
				Detail: err.Error(),
			}
		}

		setShortCookie(c.Response(), cookieState, state, env)

		if returnTo := c.QueryParam("return"); returnTo != "" {
			setShortCookie(c.Response(), cookieReturn, returnTo, env)
		}

		// `provider=...` salta la UI de Casdoor e ir directo al IdP upstream.
		// Param soportado por Casdoor (no estándar OIDC).
		authURL := p.OAuth2Cfg.AuthCodeURL(
			state,
			oauth2.SetAuthURLParam("provider", "provider_google_einar"),
		)

		return c.Redirect(http.StatusFound, authURL)
	})
}
