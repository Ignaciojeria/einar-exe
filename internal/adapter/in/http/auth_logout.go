package http

import (
	"net/http"
	"net/url"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authLogoutHandler)

// authLogoutHandler cierra la sesión local y redirige al endpoint de
// logout de Casdoor para invalidar también la sesión SSO del IdP.
//
// Si no se cierra del lado IdP, el siguiente "Login con Google" sería
// silencioso (Casdoor reusa su cookie + Google reusa la suya), lo que
// confunde al usuario.
//
// Aceptamos tanto GET como POST; lo ideal sería solo POST con CSRF token,
// pero como el id_token vive en cookie HttpOnly, GET funciona y es más
// simple para enlaces "Cerrar sesión".
func authLogoutHandler(s *fuego.Server, p *oidc.Provider, env environment.Conf) {
	handler := func(c fuego.ContextNoBody) (any, error) {
		// Borrar sesión local (id_token + refresh_token).
		clearCookie(c.Response(), cookieSession, env)
		clearCookie(c.Response(), cookieRefresh, env)

		// Redirigir a Casdoor /api/logout?post_logout_redirect_uri=...
		// para limpiar la sesión IdP. Casdoor luego volverá a APP_PUBLIC_URL.
		params := url.Values{}
		params.Set("post_logout_redirect_uri", env.APP_PUBLIC_URL+"/")

		return c.Redirect(http.StatusFound, p.LogoutURL+"?"+params.Encode())
	}

	fuego.Get(s, "/auth/logout", handler)
	fuego.Post(s, "/auth/logout", handler)
}
