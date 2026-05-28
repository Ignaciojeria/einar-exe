package http

import (
	"net/http"
	"time"

	"einar-exe/internal/adapter/out/jwtsigner"
	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"github.com/golang-jwt/jwt/v5"
)

var _ = ioc.Register(apiEmbeddedTokenHandler)

// EmbeddedTokenResponse — payload que el SPA pushea por postMessage al iframe.
type EmbeddedTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
	Issuer    string `json:"issuer"`
	JwksURI   string `json:"jwksUri"`
}

// apiEmbeddedTokenHandler — GET /api/embedded-token
//
// Mintea un JWT propio de einar con claims ricos (sub, email, tenant_id,
// tenant_slug, role). The sub is now the exe.dev user ID.
func apiEmbeddedTokenHandler(
	api *APIGroup,
	signer *jwtsigner.Signer,
	users domain.UserRepo,
	tenants domain.TenantRepo,
) {
	const tokenTTL = 10 * time.Minute

	fuego.Get(api.Server, "/embedded-token", func(c fuego.ContextNoBody) (EmbeddedTokenResponse, error) {
		ctx := c.Context()
		identity := middleware.UserFromContext(ctx)
		if identity == nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}

		var tenantID, tenantSlug, role string
		u, err := users.FindByExeDevID(ctx, identity.ExeDevUserID)
		if err != nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user lookup failed",
				Detail: err.Error(),
			}
		}
		if u.HasTenant() {
			t, err := tenants.FindByID(ctx, *u.TenantID)
			if err != nil {
				return EmbeddedTokenResponse{}, fuego.HTTPError{
					Status: http.StatusInternalServerError,
					Title:  "tenant lookup failed",
					Detail: err.Error(),
				}
			}
			tenantID = t.ID.String()
			tenantSlug = t.Slug
			role = string(u.Role)
		}

		token, exp, err := signer.Mint(jwtsigner.Claims{
			Email:      identity.Email,
			TenantID:   tenantID,
			TenantSlug: tenantSlug,
			Role:       role,
			RegisteredClaims: jwt.RegisteredClaims{Subject: identity.ExeDevUserID},
		}, "", tokenTTL)
		if err != nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not mint token",
				Detail: err.Error(),
			}
		}

		return EmbeddedTokenResponse{
			Token:     token,
			ExpiresAt: exp.Unix(),
			Issuer:    signer.Issuer(),
			JwksURI:   signer.Issuer() + "/.well-known/einar/jwks",
		}, nil
	})
}
