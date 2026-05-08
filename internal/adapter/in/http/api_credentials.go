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
	Metabase    *MetabaseCreds    `json:"metabase,omitempty"`
	// Casdoor admin se agrega cuando lleguen los iframes para gestión.
}

type OpenObserveCreds struct {
	LoginURL string `json:"loginUrl"`
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"orgId"`
}

type MetabaseCreds struct {
	LoginURL     string `json:"loginUrl"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	GroupID      int    `json:"groupId"`
	CollectionID int    `json:"collectionId"`
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

		if tenant.MetabaseGroupID != nil &&
			tenant.MetabaseCollectionID != nil &&
			tenant.MetabaseUserEmail != nil &&
			tenant.MetabaseUserPassword != nil {
			resp.Metabase = &MetabaseCreds{
				LoginURL:     "/mb/auth/login",
				Email:        *tenant.MetabaseUserEmail,
				Password:     *tenant.MetabaseUserPassword,
				GroupID:      *tenant.MetabaseGroupID,
				CollectionID: *tenant.MetabaseCollectionID,
			}
		}

		// Suprimir warning si en el futuro hay rama que no usa http.
		_ = http.StatusOK
		return resp, nil
	})
}
