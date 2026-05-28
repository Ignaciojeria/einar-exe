package http

import (
	"net/http"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiMeHandler)

// MeResponse: claims p\u00fablicos del usuario autenticado.
type MeResponse struct {
	Sub         string `json:"sub"`
	Email       string `json:"email"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Picture     string `json:"picture,omitempty"`
}

// apiMeHandler — GET /api/me
//
// El middleware ya valid\u00f3 la sesi\u00f3n; aqu\u00ed solo proyectamos los claims.
// Si llega aqu\u00ed sin user en context, es bug: el middleware deber\u00eda haber
// devuelto 401 antes.
func apiMeHandler(api *APIGroup) {
	fuego.Get(api.Server, "/me", func(c fuego.ContextNoBody) (MeResponse, error) {
		u := middleware.UserFromContext(c.Context())
		if u == nil {
			return MeResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
				Detail: "middleware bug: RequireAuth no inyect\u00f3 los claims",
			}
		}
		return claimsToMe(u), nil
	})
}

func claimsToMe(c *oidc.Claims) MeResponse {
	return MeResponse{
		Sub:         c.Sub,
		Email:       c.Email,
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Picture:     c.Picture,
	}
}
