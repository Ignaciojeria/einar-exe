// Package middleware contiene middlewares HTTP reutilizables.
//
// Auth: valida cookie de sesión OIDC o Bearer token para clientes CLI
// (PAT propio o id_token OIDC de Casdoor) y coloca claims/scopes en el context.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"einar-exe/internal/adapter/out/oidc"
	"einar-exe/internal/domain"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
)

var _ = ioc.Register(NewAuth)

const (
	cookieSession = "einar_session"
	cookieRefresh = "einar_refresh"
)

const refreshThreshold = 2 * time.Minute

type userCtxKey struct{}
type scopeCtxKey struct{}

func UserFromContext(ctx context.Context) *oidc.Claims {
	c, _ := ctx.Value(userCtxKey{}).(*oidc.Claims)
	return c
}

func withUser(ctx context.Context, c *oidc.Claims) context.Context {
	return context.WithValue(ctx, userCtxKey{}, c)
}

func withScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, scopeCtxKey{}, scopes)
}

func HasScope(ctx context.Context, scope string) bool {
	scopes, _ := ctx.Value(scopeCtxKey{}).([]string)
	for _, s := range scopes {
		if s == "*" || s == scope {
			return true
		}
	}
	return false
}

type Auth struct {
	provider *oidc.Provider
	env      environment.Conf
	users    domain.UserRepo
	tenants  domain.TenantRepo
	tokens   domain.APITokenRepo
	sf       singleflight.Group
}

func NewAuth(p *oidc.Provider, env environment.Conf, users domain.UserRepo, tenants domain.TenantRepo, tokens domain.APITokenRepo) *Auth {
	return &Auth{provider: p, env: env, users: users, tenants: tenants, tokens: tokens}
}

func (a *Auth) Require() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, scopes, err := a.authenticate(w, r)
			if err != nil {
				writeUnauthorized(w, err)
				return
			}
			ctx := withUser(r.Context(), claims)
			ctx = withScopes(ctx, scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (a *Auth) authenticate(w http.ResponseWriter, r *http.Request) (*oidc.Claims, []string, error) {
	if claims, scopes, ok := a.tryBearerToken(r); ok {
		return claims, scopes, nil
	}

	sessionCookie, err := r.Cookie(cookieSession)
	if err != nil || sessionCookie.Value == "" {
		claims, err := a.tryRefresh(w, r)
		if err != nil {
			return nil, nil, err
		}
		return claims, []string{"*"}, nil
	}

	idToken, err := a.provider.Verifier.Verify(r.Context(), sessionCookie.Value)
	if err != nil {
		claims, err := a.tryRefresh(w, r)
		if err != nil {
			return nil, nil, err
		}
		return claims, []string{"*"}, nil
	}

	if time.Until(idToken.Expiry) < refreshThreshold {
		_, _ = a.tryRefresh(w, r)
	}

	claims := &oidc.Claims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, nil, err
	}
	return claims, []string{"*"}, nil
}

func (a *Auth) tryBearerToken(r *http.Request) (*oidc.Claims, []string, bool) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return nil, nil, false
	}
	raw := strings.TrimSpace(h[len("Bearer "):])
	if raw == "" {
		return nil, nil, false
	}

	// 1) Intentar como PAT interno (hash lookup en DB).
	sum := sha256.Sum256([]byte(raw))
	tokenHash := hex.EncodeToString(sum[:])
	tok, err := a.tokens.FindActiveByHash(r.Context(), tokenHash)
	if err == nil {
		user, err := a.users.FindByID(r.Context(), tok.UserID)
		if err != nil {
			return nil, nil, false
		}
		_ = a.tokens.TouchLastUsed(r.Context(), tok.ID)

		claims := &oidc.Claims{Sub: user.CasdoorSub}
		if user.Email != nil {
			claims.Email = *user.Email
		}
		return claims, tok.Scopes, true
	}

	// 2) Fallback: intentar como id_token OIDC emitido por Casdoor
	// (útil para CLI con login PKCE sin pedir API key/PAT manual).
	idToken, err := a.provider.Verifier.Verify(r.Context(), raw)
	if err != nil {
		return nil, nil, false
	}

	claims := &oidc.Claims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, nil, false
	}
	if claims.Sub == "" {
		return nil, nil, false
	}

	var emailPtr *string
	if claims.Email != "" {
		e := claims.Email
		emailPtr = &e
	}
	user, err := a.users.EnsureBySub(r.Context(), claims.Sub, emailPtr)
	if err != nil {
		return nil, nil, false
	}
	if err := a.ensureTenantForUser(r.Context(), user, claims); err != nil {
		return nil, nil, false
	}

	// OIDC bearer obtiene permisos completos de sesión web.
	return claims, []string{"*"}, true
}

var tenantSlugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

// isSignupAllowed devuelve true si el email está en la allowlist CSV,
// o si la allowlist está vacía (modo abierto). Case-insensitive.
func isSignupAllowed(allowlistCSV, email string) bool {
	allowlistCSV = strings.TrimSpace(allowlistCSV)
	if allowlistCSV == "" {
		return true // sin allowlist configurada => modo abierto
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range strings.Split(allowlistCSV, ",") {
		if strings.ToLower(strings.TrimSpace(e)) == email {
			return true
		}
	}
	return false
}

func (a *Auth) ensureTenantForUser(ctx context.Context, user *domain.User, claims *oidc.Claims) error {
	if user.HasTenant() {
		return nil
	}

	// Anti-abuse: si hay allowlist configurada, solo emails en la lista
	// pueden crear un tenant nuevo (= consumir cuota de exe.dev).
	// Users ya con tenant existente no se ven afectados.
	if !isSignupAllowed(a.env.EINAR_SIGNUP_ALLOWLIST, claims.Email) {
		return fmt.Errorf("signup not allowed for %q: not in EINAR_SIGNUP_ALLOWLIST", claims.Email)
	}

	displayName := strings.TrimSpace(claims.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(claims.Name)
	}
	if displayName == "" {
		displayName = "My Workspace"
	}

	base := tenantBaseSlug(claims)
	lastErr := error(nil)
	for i := 0; i < 8; i++ {
		slug := base
		if i > 0 {
			suffix := fmt.Sprintf("-%d", i+1)
			if len(slug)+len(suffix) > 32 {
				slug = slug[:32-len(suffix)]
				slug = strings.Trim(slug, "-")
			}
			slug += suffix
		}

		t, err := a.tenants.Create(ctx, slug, displayName)
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				lastErr = err
				continue
			}
			return err
		}
		if err := a.users.AssignTenant(ctx, user.ID, t.ID, domain.RoleOwner); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return nil
			}
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("could not auto-provision tenant")
}

func tenantBaseSlug(claims *oidc.Claims) string {
	candidate := ""
	if at := strings.Index(claims.Email, "@"); at > 0 {
		candidate = claims.Email[:at]
	}
	if candidate == "" {
		candidate = claims.DisplayName
	}
	if candidate == "" {
		candidate = claims.Name
	}
	if candidate == "" {
		candidate = claims.Sub
	}
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	candidate = tenantSlugCleaner.ReplaceAllString(candidate, "-")
	candidate = strings.Trim(candidate, "-")
	if len(candidate) > 32 {
		candidate = candidate[:32]
		candidate = strings.Trim(candidate, "-")
	}
	if len(candidate) < 3 {
		candidate = "team-" + candidate
	}
	if len(candidate) < 3 {
		candidate = "team"
	}
	if len(candidate) > 32 {
		candidate = candidate[:32]
		candidate = strings.Trim(candidate, "-")
	}
	if strings.HasPrefix(candidate, "-") || strings.HasSuffix(candidate, "-") {
		candidate = strings.Trim(candidate, "-")
	}
	if len(candidate) < 3 {
		candidate = "team"
	}
	return candidate
}

func (a *Auth) tryRefresh(w http.ResponseWriter, r *http.Request) (*oidc.Claims, error) {
	rtCookie, err := r.Cookie(cookieRefresh)
	if err != nil || rtCookie.Value == "" {
		return nil, errors.New("no session and no refresh_token")
	}

	v, err, _ := a.sf.Do(rtCookie.Value, func() (any, error) {
		ts := a.provider.OAuth2Cfg.TokenSource(r.Context(), &oauth2.Token{RefreshToken: rtCookie.Value})
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
	a.writeFreshCookies(w, rawID, newToken.RefreshToken, idToken.Expiry)

	claims := &oidc.Claims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (a *Auth) writeFreshCookies(w http.ResponseWriter, idToken, refreshToken string, expiry time.Time) {
	secure := a.env.APP_ENV != "development" ||
		(len(a.env.APP_PUBLIC_URL) >= 8 && a.env.APP_PUBLIC_URL[:8] == "https://")

	maxAge := int(time.Until(expiry).Seconds())
	if maxAge <= 0 {
		maxAge = 3600
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSession, Value: idToken, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})

	if refreshToken != "" {
		http.SetCookie(w, &http.Cookie{Name: cookieRefresh, Value: refreshToken, Path: "/", MaxAge: 30 * 24 * 3600, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	}
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated", "detail": err.Error()})
}

var _ = (*gooidc.IDTokenVerifier)(nil)
