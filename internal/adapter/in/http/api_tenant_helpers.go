package http

import (
	"context"
	"net/http"

	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"

	"github.com/go-fuego/fuego"
)

// requireUserTenant carga el user de DB y asegura que tenga tenant_id.
//
// Centralizamos el patrón porque varios handlers de /api/* lo necesitan:
//   - /api/embedded-apps (listar / crear / borrar)
//   - cualquier endpoint scoped al tenant del user
//
// Errores tipados (fuego.HTTPError) para que el response shape sea uniforme.
//
// Devuelve (user, tenant). Si el user aún no tiene tenant, devuelve error
// 409 con instrucción al frontend de mandar a /signup.
func requireUserTenant(
	ctx context.Context,
	users domain.UserRepo,
	tenants domain.TenantRepo,
) (*domain.User, *domain.Tenant, error) {
	claims := middleware.UserFromContext(ctx)
	if claims == nil {
		return nil, nil, fuego.HTTPError{
			Status: http.StatusInternalServerError,
			Title:  "user missing in context",
		}
	}

	user, err := users.FindBySub(ctx, claims.Sub)
	if err != nil {
		return nil, nil, fuego.HTTPError{
			Status: http.StatusInternalServerError,
			Title:  "could not load user",
			Detail: err.Error(),
		}
	}
	if !user.HasTenant() {
		return nil, nil, fuego.HTTPError{
			Status: http.StatusConflict,
			Title:  "user has no tenant",
			Detail: "complete signup at /signup",
		}
	}

	tenant, err := tenants.FindByID(ctx, *user.TenantID)
	if err != nil {
		return nil, nil, fuego.HTTPError{
			Status: http.StatusInternalServerError,
			Title:  "could not load tenant",
			Detail: err.Error(),
		}
	}

	return user, tenant, nil
}

// requireOwnerOrAdmin chequea que el user.Role sea suficiente para operaciones
// de gestión del tenant (registrar embedded apps, etc.).
func requireOwnerOrAdmin(user *domain.User) error {
	if user.Role == domain.RoleOwner || user.Role == domain.RoleAdmin {
		return nil
	}
	return fuego.HTTPError{
		Status: http.StatusForbidden,
		Title:  "forbidden",
		Detail: "only owner/admin can perform this action",
	}
}
