// Package middleware contiene middlewares HTTP reutilizables.
//
// Auth: reads exe.dev proxy headers (X-ExeDev-UserID, X-ExeDev-Email)
// or Bearer token (PAT) and places user identity in the context.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewAuth)

// UserIdentity represents the authenticated user from exe.dev headers or PAT.
type UserIdentity struct {
	ExeDevUserID string
	Email        string
}

type userCtxKey struct{}
type scopeCtxKey struct{}

func UserFromContext(ctx context.Context) *UserIdentity {
	c, _ := ctx.Value(userCtxKey{}).(*UserIdentity)
	return c
}

func withUser(ctx context.Context, c *UserIdentity) context.Context {
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
	users  domain.UserRepo
	tokens domain.APITokenRepo
}

func NewAuth(users domain.UserRepo, tokens domain.APITokenRepo) *Auth {
	return &Auth{users: users, tokens: tokens}
}

func (a *Auth) Require() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, scopes, err := a.authenticate(r)
			if err != nil {
				writeUnauthorized(w, err)
				return
			}
			ctx := withUser(r.Context(), identity)
			ctx = withScopes(ctx, scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (a *Auth) authenticate(r *http.Request) (*UserIdentity, []string, error) {
	// 1. Try Bearer token (PAT) first — used by CLI.
	if identity, scopes, ok := a.tryBearerToken(r); ok {
		return identity, scopes, nil
	}

	// 2. Try exe.dev proxy headers — used by browser.
	userID := r.Header.Get("X-ExeDev-UserID")
	email := r.Header.Get("X-ExeDev-Email")
	if userID != "" {
		// Ensure user exists in DB (upsert).
		var emailPtr *string
		if email != "" {
			emailPtr = &email
		}
		if _, err := a.users.EnsureByExeDevID(r.Context(), userID, emailPtr); err != nil {
			return nil, nil, err
		}
		return &UserIdentity{
			ExeDevUserID: userID,
			Email:        email,
		}, []string{"*"}, nil
	}

	return nil, nil, errNoAuth
}

var errNoAuth = &authError{"no authentication provided (no exe.dev headers or Bearer token)"}

type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }

func (a *Auth) tryBearerToken(r *http.Request) (*UserIdentity, []string, bool) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return nil, nil, false
	}
	raw := strings.TrimSpace(h[len("Bearer "):])
	if raw == "" {
		return nil, nil, false
	}

	// PAT lookup by hash.
	sum := sha256.Sum256([]byte(raw))
	tokenHash := hex.EncodeToString(sum[:])
	tok, err := a.tokens.FindActiveByHash(r.Context(), tokenHash)
	if err != nil {
		return nil, nil, false
	}

	user, err := a.users.FindByID(r.Context(), tok.UserID)
	if err != nil {
		return nil, nil, false
	}
	_ = a.tokens.TouchLastUsed(r.Context(), tok.ID)

	identity := &UserIdentity{ExeDevUserID: user.ExeDevUserID}
	if user.Email != nil {
		identity.Email = *user.Email
	}
	return identity, tok.Scopes, true
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated", "detail": err.Error()})
}
