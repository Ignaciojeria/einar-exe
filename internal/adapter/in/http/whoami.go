package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"einar-exe/internal/adapter/out/jwtsigner"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(whoamiHandler)

// WhoamiResponse — claims públicos que devuelve /whoami.
type WhoamiResponse struct {
	Sub        string `json:"sub"`
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	TenantSlug string `json:"tenant_slug,omitempty"`
	Role       string `json:"role,omitempty"`
	Issuer     string `json:"iss,omitempty"`
	ExpiresAt  int64  `json:"exp,omitempty"`
}

// whoamiHandler — GET /whoami
//
// Endpoint PÚBLICO (sin cookie auth). Acepta `Authorization: Bearer
// <jwt>` donde el JWT debe ser uno emitido por /api/embedded-token.
//
// Pensado para devs que prefieren NO parsear JWT en su backend: hacen
// 1 request a /whoami con el token recibido y obtienen claims ya
// validados.
//
// Trade-off: round-trip extra por request vs simplicidad. Recomendado
// para prototipos / scripts. Backends serios deberían cachear o validar
// localmente con la JWKS.
//
// Errores:
//   401 — token ausente, malformado, expirado, firma inválida
//   500 — error interno
func whoamiHandler(s *fuego.Server, signer *jwtsigner.Signer) {
	s.Mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeWhoamiErr(w, http.StatusUnauthorized,
				"missing bearer token",
				"Authorization header debe ser 'Bearer <token>'")
			return
		}
		raw := strings.TrimPrefix(auth, "Bearer ")

		c, err := signer.Verify(raw, "" /* audience opcional */)
		if err != nil {
			writeWhoamiErr(w, http.StatusUnauthorized,
				"invalid token", err.Error())
			return
		}

		resp := WhoamiResponse{
			Sub:        c.Subject,
			Email:      c.Email,
			Name:       c.Name,
			TenantID:   c.TenantID,
			TenantSlug: c.TenantSlug,
			Role:       c.Role,
			Issuer:     c.Issuer,
		}
		if c.ExpiresAt != nil {
			resp.ExpiresAt = c.ExpiresAt.Unix()
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func writeWhoamiErr(w http.ResponseWriter, status int, title, detail string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  title,
		"detail": detail,
	})
}
