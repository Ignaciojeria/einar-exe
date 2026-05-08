package http

import (
	"net/http"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiCredentialsHandler)

// CredentialsResponse: secrets que el tenant necesita para acceder a
// las herramientas embebidas vía login manual o API directa.
//
// Diseño: una sección por tool. Cada una con campos planos (sin nesting
// excesivo) para que el frontend renderice copy-to-clipboard simple.
type CredentialsResponse struct {
	OpenObserve *OpenObserveCreds `json:"openobserve,omitempty"`
	// Redash, Casdoor, etc. se agregan en sus respectivas fases.
}

type OpenObserveCreds struct {
	// LoginURL: dónde el user pega las creds. Mismo dominio shell.
	LoginURL string `json:"loginUrl"`
	// Email + Password: las creds del user dedicado al tenant.
	// Si están vacíos significa que el provisioning falló o no corrió.
	// El frontend muestra "Rotar" para regenerar.
	Email    string `json:"email"`
	Password string `json:"password"`
	// OrgID: el identifier random de OO. Útil para construir API URLs:
	// https://einar.exe.xyz/o2/api/{orgId}/_search
	OrgID string `json:"orgId"`
}

// apiCredentialsHandler — GET /api/credentials
//
// Devuelve secrets del tenant. Solo accesible por owner/admin (member
// no debería ver passwords; cuando agreguemos roles más finos, un
// "viewer" tampoco).
//
// Trade-off conocido: las passwords viajan en el response body. La
// cookie HttpOnly + same-origin + TLS de exe.dev mantienen el canal
// seguro, pero el riesgo es XSS en el shell SPA. Mismo trade-off que
// /api/embedded-token.
func apiCredentialsHandler(api *APIGroup, users domain.UserRepo, tenants domain.TenantRepo) {
	fuego.Get(api.Server, "/credentials", func(c fuego.ContextNoBody) (CredentialsResponse, error) {
		ctx := c.Context()

		user, tenant, err := requireUserTenant(ctx, users, tenants)
		if err != nil {
			return CredentialsResponse{}, err
		}
		if err := requireOwnerOrAdmin(user); err != nil {
			return CredentialsResponse{}, err
		}

		var resp CredentialsResponse

		if tenant.OpenObserveOrgID != nil &&
			tenant.OpenObserveUserEmail != nil &&
			tenant.OpenObserveUserPassword != nil {
			resp.OpenObserve = &OpenObserveCreds{
				LoginURL: "/o2/web/login",
				Email:    *tenant.OpenObserveUserEmail,
				Password: *tenant.OpenObserveUserPassword,
				OrgID:    *tenant.OpenObserveOrgID,
			}
		}

		// Suprimir warning si en el futuro hay rama que no usa http.
		_ = http.StatusOK
		return resp, nil
	})
}
