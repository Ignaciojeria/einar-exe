package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"einar-exe/internal/domain"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewTenant)

// tenantCtxKey: clave privada para guardar el Tenant en context.
type tenantCtxKey struct{}

// TenantFromContext recupera el tenant resuelto por RequireTenant.
// nil si la request no pasó por el middleware o el resolver falló.
func TenantFromContext(ctx context.Context) *domain.Tenant {
	t, _ := ctx.Value(tenantCtxKey{}).(*domain.Tenant)
	return t
}

func withTenant(ctx context.Context, t *domain.Tenant) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, t)
}

// Tenant es el factory de middlewares que resuelven el tenant a partir
// del path param `{slug}` y verifican pertenencia del user autenticado.
//
// Pre-requisito: el handler ya pasó por Auth.Require() (este middleware
// lee user del context, no la cookie directamente).
type Tenant struct {
	users   domain.UserRepo
	tenants domain.TenantRepo
}

func NewTenant(users domain.UserRepo, tenants domain.TenantRepo) *Tenant {
	return &Tenant{users: users, tenants: tenants}
}

// RequireTenant devuelve un middleware que:
//
//  1. Lee `{slug}` del path (Go 1.22 r.PathValue("slug")).
//  2. Carga el tenant por slug. 404 si no existe.
//  3. Carga el user desde DB (necesita la fila, no solo el JWT) por sub.
//  4. Compara user.tenant_id con tenant.id. 403 si no matchea.
//  5. Inyecta el tenant en el context.
//
// Decisión: NO permitimos cross-tenant access aún. Owner de "acme" no
// puede ver "/t/globex/...". Si en el futuro queremos super-admins, se
// agrega un Role a nivel plataforma, no aquí.
func (t *Tenant) Require() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			if slug == "" {
				writeJSONError(w, http.StatusBadRequest, "missing tenant slug in path")
				return
			}

			claims := UserFromContext(r.Context())
			if claims == nil {
				writeJSONError(w, http.StatusInternalServerError,
					"middleware order bug: RequireTenant ran without RequireAuth")
				return
			}

			ctx := r.Context()

			tenant, err := t.tenants.FindBySlug(ctx, slug)
			if errors.Is(err, domain.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "tenant "+slug+" not found")
				return
			}
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError,
					"could not load tenant: "+err.Error())
				return
			}

			user, err := t.users.FindBySub(ctx, claims.Sub)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError,
					"could not load user: "+err.Error())
				return
			}

			if user.TenantID == nil || *user.TenantID != tenant.ID {
				// 403 (no 404) para que un user de acme sepa que existe
				// "globex" pero no tiene acceso. Si prefieres opacidad,
				// devuelve 404. Decisión consciente: 403.
				writeJSONError(w, http.StatusForbidden,
					"you do not belong to tenant "+slug)
				return
			}

			next.ServeHTTP(w, r.WithContext(withTenant(ctx, tenant)))
		})
	}
}

// writeJSONError centraliza el formato de errores del middleware.
// Mismo shape que writeUnauthorized en auth.go.
func writeJSONError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
