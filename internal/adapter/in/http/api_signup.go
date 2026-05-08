package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"einar-exe/internal/adapter/out/casdoor"
	"einar-exe/internal/adapter/out/metabase"
	"einar-exe/internal/adapter/out/openobserve"
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
func apiSignupHandler(
	api *APIGroup,
	tenants domain.TenantRepo,
	users domain.UserRepo,
	apps domain.EmbeddedAppRepo,
	cas *casdoor.Admin,
	oo *openobserve.Admin,
	mb *metabase.Admin,
) {
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

		// 5. Aprovisionar OpenObserve org. Best-effort: si falla, dejamos
		//    el tenant creado y un job de reconciliación futuro lo arregla.
		//    No queremos romper signup por una dependencia secundaria.
		provisionOpenObserve(ctx, oo, tenants, apps, tenant)
		provisionMetabase(ctx, mb, tenants, apps, tenant)

		return SignupResponse{
			TenantID:    tenant.ID.String(),
			Slug:        tenant.Slug,
			DisplayName: tenant.DisplayName,
			RedirectTo:  "/t/" + tenant.Slug + "/",
		}, nil
	})

}

// provisionOpenObserve crea la org del tenant en OO, persiste el ID
// devuelto, y registra la embedded_app system con el iframe URL.
//
// Best-effort: cualquier fallo se loggea pero NO rompe signup. Un job
// de reconciliación futuro detectará tenants con openobserve_org_id IS NULL
// y reintenta.
func provisionOpenObserve(
	ctx context.Context,
	oo *openobserve.Admin,
	tenants domain.TenantRepo,
	apps domain.EmbeddedAppRepo,
	tenant *domain.Tenant,
) {
	orgName := openobserve.SlugToOrgName(tenant.Slug)
	ooID, err := oo.CreateOrganization(ctx, orgName)
	if err != nil {
		if errors.Is(err, openobserve.ErrAlreadyExists) {
			slog.Warn("openobserve org already exists; skipping link",
				"tenant", tenant.Slug)
			return
		}
		slog.Error("could not provision OpenObserve org",
			"tenant", tenant.Slug, "err", err)
		return
	}

	if err := tenants.SetOpenObserveOrgID(ctx, tenant.ID, ooID); err != nil {
		slog.Error("created OO org but couldn't link to tenant",
			"tenant", tenant.Slug, "oo_id", ooID, "err", err)
		return
	}

	// User dedicado por tenant. Email derivado del slug (único en el
	// scope de OO). Password random; lo guardamos en plaintext por
	// ahora (ver migración 0005 para el TODO de encriptación at-rest).
	ooEmail := tenant.Slug + "@" + tenant.Slug + ".einar.local"
	ooPassword, err := openobserve.GeneratePassword()
	if err != nil {
		slog.Error("could not generate OO password", "tenant", tenant.Slug, "err", err)
		return
	}
	if err := oo.CreateUser(ctx, ooID, ooEmail, ooPassword, "admin"); err != nil {
		if !errors.Is(err, openobserve.ErrAlreadyExists) {
			slog.Error("could not create OO user for tenant",
				"tenant", tenant.Slug, "err", err)
			return
		}
		// Recovery: el user ya existía. No podemos recuperar la password
		// (OO la hashea), así que la rotamos llamando a UpdateUser. Por
		// MVP simplemente loggeamos y dejamos las creds como NULL — el user
		// puede usar "Rotar" desde la UI.
		slog.Warn("OO user already existed; credentials NOT updated",
			"tenant", tenant.Slug)
	} else {
		if err := tenants.SetOpenObserveCredentials(ctx, tenant.ID, ooEmail, ooPassword); err != nil {
			slog.Error("created OO user but couldn't persist creds",
				"tenant", tenant.Slug, "err", err)
		}
	}

	// La embedded_app system para OO. `origin` es path-relative ("/o2")
	// porque vive en el mismo dominio que el shell. El frontend detecta
	// que el origin empieza con "/" y lo trata como same-origin (no
	// dispara postMessage handshake).
	if _, err := apps.CreateSystem(ctx, tenant.ID, "OpenObserve", "/o2", nil, 10); err != nil {
		slog.Warn("could not insert OpenObserve system app",
			"tenant", tenant.Slug, "err", err)
	}

	slog.Info("openobserve provisioned",
		"tenant", tenant.Slug, "oo_id", ooID, "oo_user", ooEmail)
}

// provisionMetabase: crea group + collection + permission + user en
// Metabase, persiste IDs y credenciales en `tenants`, y registra la
// embedded_app system. Best-effort.
func provisionMetabase(
	ctx context.Context,
	mb *metabase.Admin,
	tenants domain.TenantRepo,
	apps domain.EmbeddedAppRepo,
	tenant *domain.Tenant,
) {
	groupName := metabase.SlugToGroupName(tenant.Slug)
	groupID, err := mb.CreateGroup(ctx, groupName)
	if err != nil {
		if !errors.Is(err, metabase.ErrAlreadyExists) {
			slog.Error("could not create metabase group",
				"tenant", tenant.Slug, "err", err)
			return
		}
		// Recovery: signup previo creó el group pero falló después.
		// Buscamos el ID existente y seguimos el flow.
		slog.Warn("metabase group already exists; finding existing id",
			"tenant", tenant.Slug)
		if groupID, err = mb.FindGroupByName(ctx, groupName); err != nil {
			slog.Error("metabase group exists but cannot find id",
				"tenant", tenant.Slug, "err", err)
			return
		}
	}

	collectionName := metabase.SlugToCollectionName(tenant.DisplayName)
	collectionID, err := mb.CreateCollection(ctx, collectionName, "")
	if err != nil {
		if !errors.Is(err, metabase.ErrAlreadyExists) {
			slog.Error("could not create metabase collection",
				"tenant", tenant.Slug, "err", err)
			return
		}
		// Mismo recovery que para group.
		slog.Warn("metabase collection already exists; finding existing id",
			"tenant", tenant.Slug)
		if collectionID, err = mb.FindCollectionByName(ctx, collectionName); err != nil {
			slog.Error("metabase collection exists but cannot find id",
				"tenant", tenant.Slug, "err", err)
			return
		}
	}

	if err := mb.SetCollectionPermission(ctx, collectionID, groupID, "write"); err != nil {
		// No fatal: el group puede igual ver, solo que perms quedan a
		// default (sin acceso). Se puede arreglar manual desde la UI de MB.
		slog.Warn("could not set metabase collection permission",
			"tenant", tenant.Slug, "err", err)
	}

	mbEmail := tenant.Slug + "@" + tenant.Slug + ".einar.local"
	mbPassword, err := metabase.GeneratePassword()
	if err != nil {
		slog.Error("could not generate metabase password",
			"tenant", tenant.Slug, "err", err)
		return
	}

	credsValid := true
	if _, err := mb.CreateUser(ctx, mbEmail, tenant.DisplayName, mbPassword, []int{groupID}); err != nil {
		if !errors.Is(err, metabase.ErrAlreadyExists) {
			slog.Error("could not create metabase user",
				"tenant", tenant.Slug, "err", err)
			return
		}
		// Recovery: el user ya existía. No tenemos su password (Metabase
		// no la expone), así que los creds "que conoce el shell" quedan
		// inválidos. Persistimos email vacío para que /credentials muestre
		// el card vacío con un CTA "Rotar" (futuro: Tier 2.2 de NEXT_STEPS).
		slog.Warn("metabase user already existed; creds left empty",
			"tenant", tenant.Slug, "email", mbEmail)
		credsValid = false
	}

	if credsValid {
		if err := tenants.SetMetabaseProvisioning(ctx, tenant.ID,
			groupID, collectionID, mbEmail, mbPassword); err != nil {
			slog.Error("could not persist metabase provisioning",
				"tenant", tenant.Slug, "err", err)
			return
		}
	} else {
		// Igual persistimos los IDs (group + collection) que conocemos
		// para que el sidenav muestre Metabase. Email/password vacíos.
		_ = tenants.SetMetabaseProvisioning(ctx, tenant.ID,
			groupID, collectionID, "", "")
	}

	// system embedded_app: ALWAYS, incluso si el user no se creó con
	// creds nuevas. La app aparece en sidenav y al click el iframe
	// muestra el login de Metabase para que el user use sus creds.
	if _, err := apps.CreateSystem(ctx, tenant.ID, "Metabase", "/mb", nil, 20); err != nil {
		// Conflict (ya existe) es esperado en re-signup; otros errores
		// los loggeamos pero no bloqueamos.
		slog.Warn("could not insert Metabase system app",
			"tenant", tenant.Slug, "err", err)
	}

	slog.Info("metabase provisioned",
		"tenant", tenant.Slug,
		"group_id", groupID, "collection_id", collectionID,
		"mb_user", mbEmail)
}
