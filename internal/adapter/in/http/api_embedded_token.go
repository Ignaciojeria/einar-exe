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
	// Token = JWT firmado por einar (NO por Casdoor). Lo valida el
	// backend del developer contra /.well-known/einar/jwks.
	//
	// Claims: sub, email, name, tenant_id, tenant_slug, role,
	//         iss, aud, exp, iat, nbf.
	Token string `json:"token"`

	// ExpiresAt — unix seconds. El iframe debe pedir refresh antes de esto.
	ExpiresAt int64 `json:"expiresAt"`

	// Metadata para que el dev configure su validador. También está en
	// /.well-known/einar/openid-configuration; lo embebemos por
	// conveniencia.
	Issuer  string `json:"issuer"`
	JwksURI string `json:"jwksUri"`
}

// apiEmbeddedTokenHandler — GET /api/embedded-token
//
// Endpoint same-origin con cookie HttpOnly. Mintea un JWT propio de
// einar con claims ricos (sub, email, tenant_id, tenant_slug, role).
// El SPA lo recibe y lo pushea al iframe via postMessage; el iframe
// hace `Authorization: Bearer <token>` contra el backend del dev, que
// lo valida con la public key de einar.
//
// Por qué un JWT distinto del id_token de Casdoor:
//
//  1. Claims controlables: tenant_id y role no existen en Casdoor.
//  2. Audience por embed: podemos firmar con audience específica si en
//     el futuro queremos restringir un token a una embedded app.
//  3. TTL corto (10min) con refresh transparente desde el SDK.
//  4. Rotación independiente: rotar la clave de einar NO invalida
//     sesiones del shell (que usan id_token de Casdoor).
//
// Riesgo aceptado: si el SPA sufre XSS, el atacante llama este endpoint
// y obtiene un JWT con tenant_id + role del user. Mitigación: SPA chico
// auditado, CSP, TTL corto.
func apiEmbeddedTokenHandler(
	api *APIGroup,
	signer *jwtsigner.Signer,
	users domain.UserRepo,
	tenants domain.TenantRepo,
) {
	const tokenTTL = 10 * time.Minute

	fuego.Get(api.Server, "/embedded-token", func(c fuego.ContextNoBody) (EmbeddedTokenResponse, error) {
		ctx := c.Context()
		oc := middleware.UserFromContext(ctx)
		if oc == nil {
			return EmbeddedTokenResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}

		// Resolver tenant del user. Puede no tener si está mid-signup.
		var tenantID, tenantSlug, role string
		u, err := users.FindBySub(ctx, oc.Sub)
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
			Email:      oc.Email,
			Name:       oc.Name,
			TenantID:   tenantID,
			TenantSlug: tenantSlug,
			Role:       role,
			// Subject = sub estable de Casdoor. iss/iat/exp los
			// completa el signer.
			RegisteredClaims: jwt.RegisteredClaims{Subject: oc.Sub},
		}, "" /* audience vacía; futuro: per-embed */, tokenTTL)
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
