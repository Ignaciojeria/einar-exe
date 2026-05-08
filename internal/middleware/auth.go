// Package middleware contiene middlewares HTTP reutilizables.
//
// Auth: valida la cookie `einar_session` (JWT id_token de Casdoor),
// renueva proactivamente cuando est\u00e1 cerca de expirar usando la cookie
// `einar_refresh`, e inyecta los claims del usuario en el request context.
//
// Patr\u00f3n BFF: los handlers de `/api/*` nunca tocan el token directamente;
// solo leen `middleware.UserFromContext(r.Context())`.
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
)

var _ = ioc.Register(NewAuth)

// Nombres de cookies (duplicados aqu\u00ed para no acoplar el middleware al
// paquete `http` adapter). Si cambian all\u00e1, actualizar aqu\u00ed.
const (
	cookieSession = "einar_session"
	cookieRefresh = "einar_refresh"
)

// refreshThreshold: si al id_token le quedan menos de esto, el middleware
// intenta renovarlo en background antes de servir la request.
const refreshThreshold = 2 * time.Minute

// userCtxKey es la clave privada para guardar los claims en context.Context.
// Tipo no exportado: blinda contra colisiones desde otros paquetes.
type userCtxKey struct{}

// UserFromContext recupera los claims del usuario autenticado.
// Devuelve nil si la request no pas\u00f3 por RequireAuth o si los claims se
// perdieron en alg\u00fan punto del pipeline.
func UserFromContext(ctx context.Context) *oidc.Claims {
	c, _ := ctx.Value(userCtxKey{}).(*oidc.Claims)
	return c
}

// withUser inyecta los claims en el context (helper interno).
func withUser(ctx context.Context, c *oidc.Claims) context.Context {
	return context.WithValue(ctx, userCtxKey{}, c)
}

// Auth es el middleware factory. Mant\u00e9n una sola instancia por proceso
// (vive en IoC) para reusar el `singleflight.Group`.
type Auth struct {
	provider *oidc.Provider
	env      environment.Conf
	// sf coalesce refrescos concurrentes del mismo refresh_token.
	// Sin esto, dos requests paralelas de la misma sesi\u00f3n usar\u00edan el
	// mismo refresh_token, Casdoor lo rota tras el primer uso, y el
	// segundo recibe `invalid_grant` (false-positive de logout).
	sf singleflight.Group
}

// NewAuth construye el singleton.
func NewAuth(p *oidc.Provider, env environment.Conf) *Auth {
	return &Auth{provider: p, env: env}
}

// Require devuelve el middleware. Uso:
//
//	apiGroup := fuego.Group(s, "/api", fuego.OptionMiddleware(auth.Require()))
func (a *Auth) Require() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := a.authenticate(w, r)
			if err != nil {
				writeUnauthorized(w, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), claims)))
		})
	}
}

// authenticate verifica/refresca la sesi\u00f3n y devuelve los claims.
// Side effect: si refresca, escribe nuevas cookies en `w`.
func (a *Auth) authenticate(w http.ResponseWriter, r *http.Request) (*oidc.Claims, error) {
	sessionCookie, err := r.Cookie(cookieSession)
	if err != nil || sessionCookie.Value == "" {
		// Sin id_token. \u00bfTenemos refresh_token?
		return a.tryRefresh(w, r)
	}

	idToken, err := a.provider.Verifier.Verify(r.Context(), sessionCookie.Value)
	if err != nil {
		// id_token presente pero inv\u00e1lido (firma mala, expirado, aud
		// distinto, etc.). Intentamos refresh; si falla, 401.
		return a.tryRefresh(w, r)
	}

	// Refresh proactivo: si quedan <2min, renovamos en background pero
	// servimos la request con los claims actuales (no bloqueamos).
	if time.Until(idToken.Expiry) < refreshThreshold {
		// El refresh es best-effort; ignoramos error (el id_token actual
		// todav\u00eda es v\u00e1lido por al menos unos segundos).
		_, _ = a.tryRefresh(w, r)
	}

	claims := &oidc.Claims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// tryRefresh usa el refresh_token para obtener un id_token nuevo.
// Coalesce concurrent calls v\u00eda singleflight para no quemar el
// refresh_token (Casdoor rota tras el primer uso).
func (a *Auth) tryRefresh(w http.ResponseWriter, r *http.Request) (*oidc.Claims, error) {
	rtCookie, err := r.Cookie(cookieRefresh)
	if err != nil || rtCookie.Value == "" {
		return nil, errors.New("no session and no refresh_token")
	}

	// Key = refresh_token literal: requests con la misma cookie comparten
	// el mismo Token() call.
	v, err, _ := a.sf.Do(rtCookie.Value, func() (any, error) {
		ts := a.provider.OAuth2Cfg.TokenSource(r.Context(), &oauth2.Token{
			RefreshToken: rtCookie.Value,
		})
		// .Token() detecta que no hay AccessToken/Expiry y dispara
		// el refresh_token grant contra Casdoor.
		return ts.Token()
	})
	if err != nil {
		return nil, err
	}
	newToken := v.(*oauth2.Token)

	rawID, ok := newToken.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("refresh response sin id_token")
	}

	idToken, err := a.provider.Verifier.Verify(r.Context(), rawID)
	if err != nil {
		return nil, err
	}

	// Re-setear cookies con los tokens rotados.
	a.writeFreshCookies(w, rawID, newToken.RefreshToken, idToken.Expiry)

	claims := &oidc.Claims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// writeFreshCookies escribe las cookies de sesi\u00f3n y refresh tras un
// refresh exitoso. Duplicado m\u00ednimo del adapter `http` para evitar import
// circular (middleware no debe depender de adapters).
func (a *Auth) writeFreshCookies(w http.ResponseWriter, idToken, refreshToken string, expiry time.Time) {
	secure := a.env.APP_ENV != "development" ||
		(len(a.env.APP_PUBLIC_URL) >= 8 && a.env.APP_PUBLIC_URL[:8] == "https://")

	maxAge := int(time.Until(expiry).Seconds())
	if maxAge <= 0 {
		maxAge = 3600
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieSession, Value: idToken, Path: "/",
		MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})

	if refreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name: cookieRefresh, Value: refreshToken, Path: "/",
			MaxAge: 30 * 24 * 3600, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		})
	}
}

// writeUnauthorized escribe un 401 en JSON, formato Problem+JSON ligero.
func writeUnauthorized(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "unauthenticated",
		"detail": err.Error(),
	})
}

// Sentinel para que oidc.Claims se considere "usado" sin import-cycle.
var _ = (*gooidc.IDTokenVerifier)(nil)
