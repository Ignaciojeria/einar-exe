package http

import (
	"context"
	"net/http"
	"time"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/domain"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authCallbackHandler)

// authCallbackHandler completa el flujo OAuth/OIDC.
//
// Flujo:
//  1. Valida `state` contra la cookie (mitigación CSRF).
//  2. Intercambia `code` por `id_token` server-to-server (usa CLIENT_SECRET).
//  3. Verifica firma + claims estándar del JWT contra JWKS.
//  4. Persiste el user (upsert por `casdoor_sub`).
//  5. Guarda id_token + refresh_token en cookies HttpOnly.
//  6. Redirige según estado del user:
//      - sin tenant  →  /signup     (debe completar onboarding)
//      - con tenant →  /t/{slug}/   (su workspace)
func authCallbackHandler(s *fuego.Server, p *oidc.Provider, env environment.Conf, users domain.UserRepo, tenants domain.TenantRepo) {
	fuego.Get(s, "/auth/callback", func(c fuego.ContextNoBody) (any, error) {
		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()

		// 1. Validar state.
		stateCookie, err := c.Cookie(cookieState)
		if err != nil || stateCookie.Value == "" {
			return nil, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "missing state cookie",
				Detail: "el flujo OAuth debe iniciarse desde /auth/login",
			}
		}
		if c.QueryParam("state") != stateCookie.Value {
			return nil, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "state mismatch",
				Detail: "posible intento de CSRF; el state del callback no coincide con el de la cookie",
			}
		}
		// state ya consumido: borrarlo.
		clearCookie(c.Response(), cookieState, env)

		// 2. Manejar errores que Casdoor pasa como query params.
		if oauthErr := c.QueryParam("error"); oauthErr != "" {
			return nil, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "oauth error: " + oauthErr,
				Detail: c.QueryParam("error_description"),
			}
		}

		code := c.QueryParam("code")
		if code == "" {
			return nil, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "missing code",
				Detail: "el callback no trae ?code=...",
			}
		}

		// 3. Exchange (server-to-server, usa client_secret).
		token, err := p.OAuth2Cfg.Exchange(ctx, code)
		if err != nil {
			return nil, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "token exchange failed",
				Detail: err.Error(),
			}
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || rawIDToken == "" {
			return nil, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "no id_token in response",
				Detail: "Casdoor no devolvió id_token; revisa que `tokenFormat=JWT` y `scope=openid` estén activos",
			}
		}

		// 4. Verificar firma + claims (iss, aud, exp).
		idToken, err := p.Verifier.Verify(ctx, rawIDToken)
		if err != nil {
			return nil, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "id_token invalid",
				Detail: err.Error(),
			}
		}

		var claims oidc.Claims
		if err := idToken.Claims(&claims); err != nil {
			return nil, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "could not parse id_token claims",
				Detail: err.Error(),
			}
		}
		if claims.Sub == "" {
			return nil, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "id_token without sub",
				Detail: "Casdoor emitió un token sin claim `sub`",
			}
		}

		// 4. Persistir user (upsert por sub). Idempotente: refresca email
		// si Casdoor lo cambió, no toca tenant_id ni role.
		var emailPtr *string
		if claims.Email != "" {
			e := claims.Email
			emailPtr = &e
		}
		user, err := users.EnsureBySub(ctx, claims.Sub, emailPtr)
		if err != nil {
			return nil, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "could not persist user",
				Detail: err.Error(),
			}
		}

		// 5. Sesión: id_token (corto) + refresh_token (largo).
		ttl := int(time.Until(idToken.Expiry).Seconds())
		if ttl <= 0 {
			ttl = 3600 // fallback si Casdoor manda tokens sin exp claro
		}
		setSessionCookie(c.Response(), env, rawIDToken, ttl)

		if token.RefreshToken != "" {
			// Casdoor por defecto emite refresh_tokens de larga duración.
			// 30 días alinea con el patrón estándar SaaS.
			setRefreshCookie(c.Response(), env, token.RefreshToken, 30*24*3600)
		}

		// 6. Decidir destino según tenant del user.
		//
		// Prioridad:
		//   1. cookieReturn explicit (?return=...) si existe Y el user
		//      ya tiene tenant (sino lo mandamos a completar signup primero).
		//   2. Sin tenant  → /signup
		//   3. Con tenant → /t/{slug}/
		returnTo := ""
		if cookie, err := c.Cookie(cookieReturn); err == nil && cookie.Value != "" {
			returnTo = cookie.Value
			clearCookie(c.Response(), cookieReturn, env)
		}

		if !user.HasTenant() {
			return c.Redirect(http.StatusFound, "/signup")
		}

		if returnTo == "" {
			tenant, err := tenants.FindByID(ctx, *user.TenantID)
			if err != nil {
				return nil, fuego.HTTPError{
					Status: http.StatusInternalServerError,
					Title:  "tenant not found for authenticated user",
					Detail: err.Error(),
				}
			}
			returnTo = "/t/" + tenant.Slug + "/"
		}
		return c.Redirect(http.StatusFound, returnTo)
	})
}
