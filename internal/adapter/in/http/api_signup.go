package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

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
	RedirectTo  string `json:"redirectTo"`
}

// apiSignupHandler — POST /api/signup
//
// Crea un tenant nuevo y asigna al user autenticado como owner.
// No longer provisions Casdoor organizations.
func apiSignupHandler(
	api *APIGroup,
	tenants domain.TenantRepo,
	users domain.UserRepo,
	apps domain.EmbeddedAppRepo,
	oo *openobserve.Admin,
	mb *metabase.Admin,
) {
	fuego.Post(api.Server, "/signup", func(c fuego.ContextWithBody[SignupRequest]) (SignupResponse, error) {
		identity := middleware.UserFromContext(c.Context())
		if identity == nil {
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

		body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
		body.DisplayName = strings.TrimSpace(body.DisplayName)
		if body.Slug == "" || body.DisplayName == "" {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusBadRequest,
				Title:  "slug and displayName are required",
			}
		}

		ctx := c.Context()

		user, err := users.FindByExeDevID(ctx, identity.ExeDevUserID)
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

		// 2. Asignar al user como owner.
		if err := users.AssignTenant(ctx, user.ID, tenant.ID, domain.RoleOwner); err != nil {
			return SignupResponse{}, fuego.HTTPError{
				Status: http.StatusInternalServerError,
				Title:  "tenant created but user assignment failed",
				Detail: err.Error(),
			}
		}

		// 3. Aprovisionar OpenObserve org. Best-effort.
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
		slog.Warn("OO user already existed; credentials NOT updated",
			"tenant", tenant.Slug)
	} else {
		if err := tenants.SetOpenObserveCredentials(ctx, tenant.ID, ooEmail, ooPassword); err != nil {
			slog.Error("created OO user but couldn't persist creds",
				"tenant", tenant.Slug, "err", err)
		}
	}

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
		slog.Warn("metabase collection already exists; finding existing id",
			"tenant", tenant.Slug)
		if collectionID, err = mb.FindCollectionByName(ctx, collectionName); err != nil {
			slog.Error("metabase collection exists but cannot find id",
				"tenant", tenant.Slug, "err", err)
			return
		}
	}

	if err := mb.SetCollectionPermission(ctx, collectionID, groupID, "write"); err != nil {
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
		_ = tenants.SetMetabaseProvisioning(ctx, tenant.ID,
			groupID, collectionID, "", "")
	}

	if _, err := apps.CreateSystem(ctx, tenant.ID, "Metabase", "/mb", nil, 20); err != nil {
		slog.Warn("could not insert Metabase system app",
			"tenant", tenant.Slug, "err", err)
	}

	slog.Info("metabase provisioned",
		"tenant", tenant.Slug,
		"group_id", groupID, "collection_id", collectionID,
		"mb_user", mbEmail)
}
