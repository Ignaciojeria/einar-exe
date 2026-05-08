// JWT validation middleware contra einar JWKS.
//
// Usa github.com/coreos/go-oidc/v3, la misma lib que einar usa
// internamente. Cachea la JWKS automáticamente (refetch on-cache-miss
// cuando aparece un kid desconocido).
package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Claims son los campos que einar mete en el JWT.
type Claims struct {
	Subject    string    `json:"sub"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	TenantID   string    `json:"tenant_id"`
	TenantSlug string    `json:"tenant_slug"`
	Role       string    `json:"role"`
	IssuedAt   time.Time `json:"-"`
	ExpiresAt  time.Time `json:"-"`
}

type Auth struct {
	verifier *oidc.IDTokenVerifier
}

// NewAuth construye el verificador. Hace fetch del JWKS al boot.
func NewAuth(issuer, jwksURI string) (*Auth, error) {
	ctx := context.Background()

	// go-oidc espera un discovery doc estándar OIDC. einar expone uno
	// minimal en /.well-known/einar/openid-configuration. Lo cargamos
	// manualmente con KeySet directo para no depender del discovery.
	keySet := oidc.NewRemoteKeySet(ctx, jwksURI)

	verifier := oidc.NewVerifier(issuer, keySet, &oidc.Config{
		// einar no setea audience por ahora; deshabilitamos chequeo.
		// Cuando esté disponible, poné aquí el origin de tu app.
		SkipClientIDCheck: true,
	})

	return &Auth{verifier: verifier}, nil
}

type contextKey struct{}

var claimsKey = contextKey{}

// Middleware valida el header Authorization: Bearer <jwt>.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeAuthErr(w, "missing bearer token")
			return
		}
		raw := strings.TrimPrefix(authHeader, "Bearer ")

		idToken, err := a.verifier.Verify(r.Context(), raw)
		if err != nil {
			writeAuthErr(w, "invalid token: "+err.Error())
			return
		}

		var c Claims
		if err := idToken.Claims(&c); err != nil {
			writeAuthErr(w, "could not parse claims: "+err.Error())
			return
		}
		c.Subject = idToken.Subject
		c.IssuedAt = idToken.IssuedAt
		c.ExpiresAt = idToken.Expiry

		ctx := context.WithValue(r.Context(), claimsKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthFromContext devuelve los claims del request validado, o nil.
func AuthFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// MustAuth devuelve los claims o panic — útil cuando ya pasó el middleware.
func MustAuth(ctx context.Context) *Claims {
	c := AuthFromContext(ctx)
	if c == nil {
		panic(errors.New("no auth in context — middleware no aplicado?"))
	}
	return c
}

func writeAuthErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
