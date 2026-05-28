package http

import (
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(NewAPIGroup)

// APIGroup envuelve el sub-server de fuego que sirve `/api/*` con el
// middleware `RequireAuth` aplicado a todas sus rutas.
type APIGroup struct {
	*fuego.Server
}

// NewAPIGroup crea el grupo /api con auth middleware aplicado.
func NewAPIGroup(s *fuego.Server, auth *middleware.Auth) *APIGroup {
	g := fuego.Group(s, "/api", fuego.OptionMiddleware(auth.Require()))
	return &APIGroup{Server: g}
}
