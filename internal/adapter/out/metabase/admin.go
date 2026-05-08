// Package metabase encapsula calls server-to-server a la admin API.
//
// Auth: Metabase usa session tokens (POST /api/session retorna `id`).
// Cacheamos el token y lo refrescamos cuando responde 401.
//
// Provisioning per-tenant pattern:
//   1. CreateGroup("tenant-{slug}")            → permission group
//   2. CreateCollection("Acme")                → folder de dashboards
//   3. SetCollectionPermissions(collection, group, "read+write")
//   4. CreateUser(email, password, [groupID])  → user del tenant
//
// Los IDs (group, collection) se persisten en `tenants` para luego
// borrar cuando se de-aprovisione un tenant.
package metabase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewAdmin)

var (
	ErrAlreadyExists = errors.New("metabase: object already exists")
	ErrUnauthorized  = errors.New("metabase: admin login rejected")
)

// Admin es el cliente HTTP a Metabase. Singleton via IoC.
type Admin struct {
	baseURL  string
	email    string
	password string
	http     *http.Client

	mu      sync.Mutex
	session string // X-Metabase-Session token
}

func NewAdmin(env environment.Conf) *Admin {
	return &Admin{
		baseURL:  strings.TrimRight(env.METABASE_ENDPOINT_INTERNAL, "/"),
		email:    env.METABASE_ADMIN_EMAIL,
		password: env.METABASE_ADMIN_PASSWORD,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// login obtiene un session token. Reusa el cacheado si existe; si la
// próxima request da 401, llamamos `invalidateSession` y se re-hace.
func (a *Admin) login(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session != "" {
		return a.session, nil
	}
	body, _ := json.Marshal(map[string]string{
		"username": a.email,
		"password": a.password,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/api/session", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("metabase login: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("%w: %s", ErrUnauthorized, string(raw))
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("metabase login %d: %s", resp.StatusCode, string(raw))
	}
	var ok struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return "", fmt.Errorf("metabase login parse: %w", err)
	}
	if ok.ID == "" {
		return "", fmt.Errorf("metabase login: no session id in response: %s", string(raw))
	}
	a.session = ok.ID
	return a.session, nil
}

func (a *Admin) invalidateSession() {
	a.mu.Lock()
	a.session = ""
	a.mu.Unlock()
}

// do envía una request autenticada con session token. Reintenta una vez
// si recibe 401 (sesión expirada).
func (a *Admin) do(ctx context.Context, method, path string, body any, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := a.login(ctx)
		if err != nil {
			return err
		}
		var reqBody io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reqBody = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, reqBody)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Metabase-Session", token)

		resp, err := a.http.Do(req)
		if err != nil {
			return fmt.Errorf("metabase %s %s: %w", method, path, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			// session expiró; renovar y reintentar.
			a.invalidateSession()
			continue
		}
		if resp.StatusCode == http.StatusConflict ||
			(resp.StatusCode >= 400 && resp.StatusCode < 500 &&
				bytesContainsFold(raw, []byte("already"))) {
			return ErrAlreadyExists
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("metabase %s %s -> %d: %s",
				method, path, resp.StatusCode, string(raw))
		}
		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return fmt.Errorf("metabase %s %s parse: %w; raw: %s",
					method, path, err, string(raw))
			}
		}
		return nil
	}
	return fmt.Errorf("metabase: session refresh failed twice")
}

// CreateGroup crea un permission group y devuelve su ID.
// Si el name ya existe, devuelve ErrAlreadyExists.
func (a *Admin) CreateGroup(ctx context.Context, name string) (int, error) {
	var resp struct {
		ID int `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, "/api/permissions/group",
		map[string]string{"name": name}, &resp); err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// CreateCollection crea una collection (folder de dashboards/queries)
// y devuelve su ID.
func (a *Admin) CreateCollection(ctx context.Context, name, color string) (int, error) {
	if color == "" {
		color = "#509EE3" // azul Metabase por default
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, "/api/collection",
		map[string]any{
			"name":        name,
			"color":       color,
			"description": "Auto-created for tenant " + name,
		}, &resp); err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// SetCollectionPermission da `level` ("read" | "write" | "none") al
// `groupID` sobre la `collectionID`.
//
// Metabase usa un endpoint "graph" donde mandás el ENTERO mapa de
// permisos. Para evitar leer-modificar-escribir, usamos el endpoint
// PUT /api/collection/graph con un partial — Metabase mergea.
func (a *Admin) SetCollectionPermission(ctx context.Context, collectionID, groupID int, level string) error {
	// El graph endpoint requiere `revision`. Lo obtenemos primero.
	var current struct {
		Revision int                       `json:"revision"`
		Groups   map[string]map[string]any `json:"groups"`
	}
	if err := a.do(ctx, http.MethodGet, "/api/collection/graph", nil, &current); err != nil {
		return err
	}

	// Mergear nuestro cambio.
	groupKey := fmt.Sprintf("%d", groupID)
	collKey := fmt.Sprintf("%d", collectionID)
	if current.Groups[groupKey] == nil {
		current.Groups[groupKey] = map[string]any{}
	}
	current.Groups[groupKey][collKey] = level

	return a.do(ctx, http.MethodPut, "/api/collection/graph", current, nil)
}

// CreateUser crea un user, lo asigna a los `groupIDs` indicados y
// retorna su ID. La password se manda plaintext sobre la red interna
// docker; Metabase la hashea (bcrypt).
func (a *Admin) CreateUser(ctx context.Context, email, firstName, password string, groupIDs []int) (int, error) {
	body := map[string]any{
		"email":      email,
		"first_name": firstName,
		"last_name":  "-",
		"password":   password,
		"group_ids":  groupIDs,
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, "/api/user", body, &resp); err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// SlugToGroupName convierte slug del tenant a nombre del group.
// Prefijo "tenant-" para que un admin de Metabase distinga rápido entre
// groups del sistema (Administrators, "All Users") y los nuestros.
func SlugToGroupName(slug string) string {
	return "tenant-" + slug
}

// SlugToCollectionName: nombre humano-friendly de la collection.
func SlugToCollectionName(displayName string) string {
	return displayName
}

// GeneratePassword: hex de 32 chars (128 bits).
func GeneratePassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func bytesContainsFold(s, sub []byte) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				continue outer
			}
		}
		return true
	}
	return false
}
