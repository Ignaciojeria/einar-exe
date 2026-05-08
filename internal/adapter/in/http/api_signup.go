package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"einar-exe/internal/adapter/out/casdoor"
	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(apiSignupHandler)

// SignupRequest: body para POST /api/signup.
type SignupRequest struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
}

// SignupResponse: tenant recién creado + flag para redirigir.
type SignupResponse struct {
	TenantID    string `json:"tenantId"`
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	// RedirectTo: URL absoluta-path al workspace del tenant.
	// El frontend hace window.location = redirectTo tras el signup.
	RedirectTo string `json:"redirectTo"`
}

// apiSignupHandler — POST /api/signup
//
// Crea un tenant nuevo, aprovisiona la organization correspondiente en
// Casdoor, y asigna al user autenticado como owner.
//
// Reglas:
//   - User debe estar autenticado (middleware ya validó la cookie).
//   - User NO debe tener tenant todavía (un email = un tenant en MVP).
//   - Slug debe pasar el CHECK constraint: ^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$
//
// Transaccionalidad pragmática:
//   1. INSERT tenant en Postgres
//   2. POST /api/add-organization en Casdoor
//   3. UPDATE tenant.casdoor_org en Postgres
//   4. UPDATE user.tenant_id + role en Postgres
//
// Si (2) falla → rollback de (1) (DELETE FROM tenants).
// Si (3) falla → dejamos el org en Casdoor pero el tenant queda sin
//                link; un job de reconciliación futuro lo arregla.
// Si (4) falla → idem; el user puede reintentar /api/signup y caer en
//                ErrAlreadyExists del slug, que tratamos como recovery.
func apiSignupHandler(api *APIGroup, tenants domain.TenantRepo, users domain.UserRepo, cas *casdoor.Admin) {
	fuego.Post(api.Server, "/signup", func(c fuego.ContextWithBody[SignupRequest]) (SignupResponse, error) {
		claims := middleware.UserFromContext(c.Context())
		if claims == nil {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user missing in context",
			}
		}

		body, err := c.Body()
		if err != nil {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "invalid body",
				Detail: err.Error(),
			}
		}

		// Normalizar inputs antes de validar.
		body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
		body.DisplayName = strings.TrimSpace(body.DisplayName)
		if body.Slug == "" || body.DisplayName == "" {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "slug and displayName are required",
			}
		}

		ctx := c.Context()

		// Recuperar el user de DB. EnsureBySub ya corrió en /auth/callback,
		// pero por seguridad lo invocamos por si alguien postea sin haber
		// pasado por callback (ej. flujo manual con cookie de otra sesión).
		user, err := users.FindBySub(ctx, claims.Sub)
		if err != nil {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "user not found in DB",
				Detail: err.Error(),
			}
		}
		if user.HasTenant() {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusConflict,
				Title:  "user already belongs to a tenant",
			}
		}

		// 1. Crear tenant en Postgres.
		tenant, err := tenants.Create(ctx, body.Slug, body.DisplayName)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrConflict):
				return SignupResponse{}, fuego.HTTPError{
					Status: http.StatusConflict,
					Title:  "slug already taken",
				}
			case errors.Is(err, domain.ErrInvalidArg):
				return SignupResponse{}, fuego.HTTPError{
					Status: http.StatusBadRequest,
					Title:  "invalid slug format",
					Detail: "slug must be 3-32 chars, lowercase, alphanumeric + dashes",
				}
			default:
				return SignupResponse{}, fuego.HTTPError{
					Status: http.StatusInternalServerError,
					Title:  "could not create tenant",
					Detail: err.Error(),
				}
			}
		}

		// 2. Aprovisionar org en Casdoor.
		if err := cas.CreateOrganization(ctx, tenant.Slug, tenant.DisplayName); err != nil {
			if !errors.Is(err, casdoor.ErrAlreadyExists) {
				// Rollback del tenant. Usamos un context separado por si el
				// error original fue un timeout: queremos que el DELETE
				// tenga su propio chance.
				delCtx, cancel := context.WithTimeout(context.Background(), 3*1e9)
				defer cancel()
				if derr := tenants.Delete(delCtx, tenant.ID); derr != nil {
					slog.Error("signup rollback failed",
						"tenantID", tenant.ID, "err", derr)
				}
				return SignupResponse{}, fuego.HTTPError{
					Status: http.StatusBadGateway,
					Title:  "could not provision casdoor organization",
					Detail: err.Error(),
				}
			}
			// AlreadyExists = recovery de un signup previo que se cayó entre
			// step 2 y 3. Continuamos.
			slog.Warn("casdoor org already exists, treating as recovery",
				"slug", tenant.Slug)
		}

		// 3. Linkear tenant <-> casdoor_org.
		if err := tenants.SetCasdoorOrg(ctx, tenant.ID, tenant.Slug); err != nil {
			slog.Error("could not link casdoor_org; manual reconcile needed",
				"tenantID", tenant.ID, "slug", tenant.Slug, "err", err)
			// No fallamos: el org existe en Casdoor y el tenant en DB.
			// El campo casdoor_org se puede setear después con un job.
		}

		// 4. Asignar al user como owner.
		if err := users.AssignTenant(ctx, user.ID, tenant.ID, domain.RoleOwner); err != nil {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "tenant created but user assignment failed",
				Detail: err.Error(),
			}
		}

		return SignupResponse{
			TenantID:    tenant.ID.String(),
			Slug:        tenant.Slug,
			DisplayName: tenant.DisplayName,
			RedirectTo:  "/t/" + tenant.Slug + "/",
		}, nil
	})
}
