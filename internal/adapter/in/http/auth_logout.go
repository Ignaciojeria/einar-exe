package http

import (
	"net/http"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authLogoutHandler)

// authLogoutHandler — GET/POST /auth/logout
//
// Redirects to exe.dev’s logout endpoint which clears the exe.dev
// session cookie and returns the user to the app.
func authLogoutHandler(s *fuego.Server) {
	handler := func(c fuego.ContextNoBody) (any, error) {
		return c.Redirect(http.StatusFound, "/__exe.dev/logout")
	}

	fuego.Get(s, "/auth/logout", handler)
	fuego.Post(s, "/auth/logout", handler)
}
