// Package oidc encapsula la integración con el OpenID Provider (Casdoor).
//
// Diseño:
//   - Singleton creado una vez al arrancar (vía IoC).
//   - Discovery automático contra `CASDOOR_ENDPOINT_INTERNAL` (red Docker,
//     sin TLS, rápido).
//   - Verificación de JWT contra `iss = CASDOOR_ORIGIN` (URL pública), que
//     es lo que firma Casdoor cuando un browser inicia el flujo. Esto se
//     resuelve con `oidc.InsecureIssuerURLContext`.
//   - JWKS dinámico: la librería refresca claves automáticamente.
//
// Ver docs/casdoor-integration-plan.md §12bis.
package oidc

import (
	"context"
	"fmt"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var _ = ioc.Register(NewProvider)

// Provider agrupa los objetos derivados del discovery OIDC, listos para
// usar desde los handlers de auth.
type Provider struct {
	// Provider expone los endpoints del IdP (auth, token, userinfo, jwks).
	Provider *gooidc.Provider

	// OAuth2Cfg encapsula client_id/secret + endpoints + redirect URI.
	// Lo usamos para construir la URL de authorize y para hacer el
	// `Exchange(code)`.
	OAuth2Cfg *oauth2.Config

	// Verifier valida la firma y los claims estándar (`iss`, `aud`,
	// `exp`, `iat`) del id_token recibido en el callback.
	Verifier *gooidc.IDTokenVerifier

	// LogoutURL absoluto al endpoint de Casdoor que cierra la sesión SSO.
	// Casdoor expone `/api/logout` que invalida la sesión del lado IdP.
	LogoutURL string
}

// NewProvider hace discovery + arma OAuth2.Config + Verifier.
//
// Arquitectura de URLs (CRÍTICO en setups detrás de un edge proxy como exe.dev):
//
//	┌─────────────────────────┬────────────────────────────────────────┐
//	│ Para qué                │ URL                                    │
//	├─────────────────────────┼────────────────────────────────────────┤
//	│ Discovery (server)      │ http://casdoor:8000   (docker DNS)     │
//	│ Authorize (browser)     │ https://einar.exe.xyz:8000  (public)   │
//	│ Token exchange (server) │ http://casdoor:8000   (docker DNS)     │
//	│ JWKS (server)           │ http://casdoor:8000   (docker DNS)     │
//	│ iss del JWT             │ https://einar.exe.xyz:8000  (public)   │
//	└─────────────────────────┴────────────────────────────────────────┘
//
// El browser usa URLs públicas para que el flujo OAuth cierre con
// cookies del dominio público. Toda llamada desde el backend usa el
// hostname interno de Docker para no tener que salir por el edge
// (que pediría autenticación de exe.dev y rompería el flujo).
//
// `InsecureIssuerURLContext` le dice a la librería: "el iss del JWT es
// la URL pública aunque haya descargado el discovery por la URL interna".
func NewProvider(env environment.Conf) (*Provider, error) {
	ctx := gooidc.InsecureIssuerURLContext(context.Background(), env.CASDOOR_ORIGIN)

	provider, err := gooidc.NewProvider(ctx, env.CASDOOR_ENDPOINT_INTERNAL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery against %s: %w",
			env.CASDOOR_ENDPOINT_INTERNAL, err)
	}

	// AuthURL = público (browser); TokenURL = interno (server-to-server).
	// Ignoramos provider.Endpoint() a propósito: ese trae ambas como públicas.
	oauth2Cfg := &oauth2.Config{
		ClientID:     env.CASDOOR_CLIENT_ID,
		ClientSecret: env.CASDOOR_CLIENT_SECRET,
		Endpoint: oauth2.Endpoint{
			AuthURL:  env.CASDOOR_ORIGIN + "/login/oauth/authorize",
			TokenURL: env.CASDOOR_ENDPOINT_INTERNAL + "/api/login/oauth/access_token",
		},
		RedirectURL: env.APP_OAUTH_REDIRECT_URI,
		Scopes:      []string{gooidc.ScopeOpenID, "profile", "email"},
	}

	// Verifier con JWKS por la red interna (no por el edge público).
	// Construimos KeySet manualmente apuntando al endpoint interno;
	// el `iss` esperado sigue siendo el público para que matchee el JWT.
	internalJWKS := env.CASDOOR_ENDPOINT_INTERNAL + "/.well-known/jwks"
	keySet := gooidc.NewRemoteKeySet(context.Background(), internalJWKS)
	verifier := gooidc.NewVerifier(
		env.CASDOOR_ORIGIN, // expected iss
		keySet,
		&gooidc.Config{ClientID: env.CASDOOR_CLIENT_ID},
	)

	return &Provider{
		Provider:  provider,
		OAuth2Cfg: oauth2Cfg,
		Verifier:  verifier,
		LogoutURL: env.CASDOOR_ORIGIN + "/api/logout",
	}, nil
}

// Claims son los campos del id_token que el resto de la app necesita.
// El `Sub` es el identificador estable del usuario en Casdoor (lo que
// se persiste en `users.casdoor_sub`).
type Claims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	// Casdoor-specific (no estándar OIDC pero útil):
	DisplayName       string `json:"displayName"`
	SignupApplication string `json:"signupApplication"`
}
