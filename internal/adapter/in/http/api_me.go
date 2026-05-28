package http

import (
	"net/http"

	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiMeHandler)

// MeResponse: identity of the authenticated user.
type MeResponse struct {
	ExeDevUserID string `json:"exeDevUserId"`
	Email        string `json:"email"`
}

// apiMeHandler — GET /api/me
func apiMeHandler(api *APIGroup) {
	fuego.Get(api.Server, "/me", func(c fuego.ContextNoBody) (MeResponse, error) {
		u := middleware.UserFromContext(c.Context())
		if u == nil {
			return MeResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
				Detail: "middleware bug: RequireAuth did not inject identity",
			}
		}
		return MeResponse{
			ExeDevUserID: u.ExeDevUserID,
			Email:        u.Email,
		}, nil
	})
}
