package http

import (
	"net/http"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authLoginHandler)

// authLoginHandler — GET /auth/login
//
// Redirects to exe.dev’s login endpoint. After authentication, exe.dev
// redirects back to the specified return path.
func authLoginHandler(s *fuego.Server) {
	fuego.Get(s, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		returnTo := c.QueryParam("return")
		if returnTo == "" {
			returnTo = "/"
		}
		return c.Redirect(http.StatusFound, "/__exe.dev/login?redirect="+returnTo)
	})
}
