package http

import (
	"errors"
	"net/http"
	"time"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"golang.org/x/oauth2"
)

var _ = ioc.Register(authRefreshHandler)

// RefreshResponse confirma que la sesi\u00f3n fue renovada.
// El nuevo id_token NO se devuelve en el body: viaja por cookie HttpOnly.
type RefreshResponse struct {
	Refreshed bool      `json:"refreshed"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// authRefreshHandler \u2014 POST /auth/refresh
//
// Renueva expl\u00edcitamente la sesi\u00f3n usando la cookie `einar_refresh`.
// El middleware `RequireAuth` ya hace refresh transparente cuando una
// request a `/api/*` llega con id_token cerca de expirar; este endpoint
// existe para casos donde el frontend quiere forzar el refresh antes de
// alguna operaci\u00f3n cr\u00edtica (ej. abrir un iframe que recibir\u00e1 el token).
//
// POST (no GET) porque cambia estado del servidor (rotaci\u00f3n de
// refresh_token en Casdoor).
func authRefreshHandler(s *fuego.Server, p *oidc.Provider, env environment.Conf) {
	fuego.Post(s, "/auth/refresh", func(c fuego.ContextNoBody) (RefreshResponse, error) {
		rt, err := c.Cookie(cookieRefresh)
		if err != nil || rt.Value == "" {
			return RefreshResponse{}, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "no refresh_token",
				Detail: "no hay cookie de refresh; el usuario debe re-loguear",
			}
		}

		ts := p.OAuth2Cfg.TokenSource(c.Context(), &oauth2.Token{
			RefreshToken: rt.Value,
		})
		newToken, err := ts.Token()
		if err != nil {
			// invalid_grant = refresh_token revocado/expirado/ya rotado.
			// Borramos cookies para forzar nuevo login.
			clearCookie(c.Response(), cookieSession, env)
			clearCookie(c.Response(), cookieRefresh, env)
			return RefreshResponse{}, fuego.HTTPError{
				Status: http.StatusUnauthorized,
				Title:  "refresh failed",
				Detail: err.Error(),
			}
		}

		rawID, ok := newToken.Extra("id_token").(string)
		if !ok || rawID == "" {
			return RefreshResponse{}, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "refresh response sin id_token",
			}
		}

		idToken, err := p.Verifier.Verify(c.Context(), rawID)
		if err != nil {
			return RefreshResponse{}, fuego.HTTPError{
				Status: http.StatusBadGateway,
				Title:  "id_token inv\u00e1lido tras refresh",
				Detail: err.Error(),
			}
		}

		ttl := int(time.Until(idToken.Expiry).Seconds())
		if ttl <= 0 {
			ttl = 3600
		}
		setSessionCookie(c.Response(), env, rawID, ttl)
		if newToken.RefreshToken != "" {
			setRefreshCookie(c.Response(), env, newToken.RefreshToken, 30*24*3600)
		}

		return RefreshResponse{
			Refreshed: true,
			ExpiresAt: idToken.Expiry,
		}, nil
	})
}

// _ ensures errors import is used even if linter strips it.
var _ = errors.New
