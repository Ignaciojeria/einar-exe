package http

import (
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(NewAPIGroup)

// APIGroup envuelve el sub-server de fuego que sirve `/api/*` con el
// middleware `RequireAuth` aplicado a todas sus rutas.
//
// Tipo distinto a `*fuego.Server` para que IoC los inyecte de forma
// independiente: handlers p\u00fablicos toman `*fuego.Server`, handlers
// protegidos toman `*APIGroup`.
type APIGroup struct {
	*fuego.Server
}

// NewAPIGroup crea el grupo /api con auth middleware aplicado.
//
// Cualquier handler que se registre con `fuego.Get(api.Server, ...)` o
// usando `api.Server` queda autom\u00e1ticamente protegido.
func NewAPIGroup(s *fuego.Server, auth *middleware.Auth) *APIGroup {
	g := fuego.Group(s, "/api", fuego.OptionMiddleware(auth.Require()))
	return &APIGroup{Server: g}
}
