package http

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var _ = ioc.Register(apiTokensHandler)

type TokenCreateRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type TokenCreateResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	Token     string     `json:"token"`
}

type TokenDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"tokenPrefix"`
	Scopes      []string   `json:"scopes"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func apiTokensHandler(api *APIGroup, users domain.UserRepo, tokens domain.APITokenRepo) {
	fuego.Post(api.Server, "/tokens", func(c fuego.ContextWithBody[TokenCreateRequest]) (TokenCreateResponse, error) {
		body, err := c.Body()
		if err != nil {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusBadRequest, Title: "invalid body", Detail: err.Error()}
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusBadRequest, Title: "name is required"}
		}
		if len(body.Scopes) == 0 {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusBadRequest, Title: "scopes are required"}
		}

		identity := middleware.UserFromContext(c.Context())
		if identity == nil {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "user missing in context"}
		}
		user, err := users.FindByExeDevID(c.Context(), identity.ExeDevUserID)
		if err != nil {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not load user", Detail: err.Error()}
		}

		raw, prefix, hash, err := generatePAT()
		if err != nil {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not generate token", Detail: err.Error()}
		}

		t, err := tokens.Create(c.Context(), user.ID, body.Name, prefix, hash, body.Scopes, body.ExpiresAt)
		if err != nil {
			return TokenCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not create token", Detail: err.Error()}
		}

		return TokenCreateResponse{ID: t.ID.String(), Name: t.Name, Scopes: t.Scopes, ExpiresAt: t.ExpiresAt, CreatedAt: t.CreatedAt, Token: raw}, nil
	})

	fuego.Get(api.Server, "/tokens", func(c fuego.ContextNoBody) (map[string][]TokenDTO, error) {
		identity := middleware.UserFromContext(c.Context())
		if identity == nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "user missing in context"}
		}
		user, err := users.FindByExeDevID(c.Context(), identity.ExeDevUserID)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not load user", Detail: err.Error()}
		}
		list, err := tokens.ListByUser(c.Context(), user.ID)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not list tokens", Detail: err.Error()}
		}
		out := make([]TokenDTO, 0, len(list))
		for _, t := range list {
			out = append(out, TokenDTO{ID: t.ID.String(), Name: t.Name, TokenPrefix: t.TokenPrefix, Scopes: t.Scopes, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt, CreatedAt: t.CreatedAt})
		}
		return map[string][]TokenDTO{"tokens": out}, nil
	})

	fuego.DeleteStd(api.Server, "/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid id", err.Error())
			return
		}
		identity := middleware.UserFromContext(r.Context())
		if identity == nil {
			writeAPIError(w, http.StatusInternalServerError, "user missing in context", "")
			return
		}
		user, err := users.FindByExeDevID(r.Context(), identity.ExeDevUserID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "could not load user", err.Error())
			return
		}
		if err := tokens.Revoke(r.Context(), id, user.ID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeAPIError(w, http.StatusNotFound, "token not found", "")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "could not revoke token", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func generatePAT() (raw, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	raw = "pat_" + hex.EncodeToString(b)
	prefix = raw[:12]
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, prefix, hash, nil
}
