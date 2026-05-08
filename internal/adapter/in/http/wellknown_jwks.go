package http

import (
	"encoding/json"
	"net/http"

	"einar-exe/internal/adapter/out/jwtsigner"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(wellKnownJWKSHandler)

// wellKnownJWKSHandler — GET /.well-known/einar/jwks
//
// Endpoint PÚBLICO (sin auth) que devuelve la clave pública del signer
// einar en formato JWKS. Cualquier backend tercero que reciba un JWT
// emitido por /api/embedded-token lo valida fetchando este endpoint y
// verificando la firma con la pública.
//
// Estable: el `kid` cambia solo si rota la clave (no en cada arranque
// si EINAR_JWT_PRIVATE_KEY está seteada).
//
// Cacheable agresivamente: clientes pueden cachear por horas. Si rotás
// la clave, el cliente verá un kid distinto en el JWT y deberá refetchear.
//
// También exponemos /.well-known/einar/openid-configuration con un
// discovery doc minimal para que libs como jose/go-oidc lo encuentren
// automáticamente.
func wellKnownJWKSHandler(s *fuego.Server, signer *jwtsigner.Signer) {
	// JWKS
	s.Mux.HandleFunc("GET /.well-known/einar/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(signer.JWKS())
	})

	// Discovery doc minimal. No es un OIDC provider full (no hay
	// authorization_endpoint, etc) — solo lo necesario para que libs
	// hagan introspection automática.
	s.Mux.HandleFunc("GET /.well-known/einar/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                signer.Issuer(),
			"jwks_uri":                              signer.Issuer() + "/.well-known/einar/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"subject_types_supported":               []string{"public"},
			// Convención propia de einar para que devs sepan validar:
			"einar_whoami_endpoint": signer.Issuer() + "/whoami",
		})
	})
}
