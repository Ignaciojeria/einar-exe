// Package openobserve encapsula calls server-to-server a la admin API
// de OpenObserve. Se usa en el flujo de signup para aprovisionar
// una organization por tenant.
//
// Auth: Basic Auth con `ZO_ROOT_USER_EMAIL:ZO_ROOT_USER_PASSWORD`.
//
// Restricción que conviene saber: OO solo acepta nombres de org con
// caracteres alfanuméricos, espacios y underscores. Los slugs de
// nuestros tenants pueden tener guiones; los traducimos a underscores
// antes de enviarlos.
package openobserve

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
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewAdmin)

var (
	ErrAlreadyExists = errors.New("openobserve: organization already exists")
	ErrUnauthorized  = errors.New("openobserve: admin credentials rejected")
)

// Admin: cliente HTTP a OpenObserve. Singleton via IoC.
type Admin struct {
	baseURL  string // http://openobserve:5080 (interno docker)
	user     string
	password string
	http     *http.Client
}

// NewAdmin lee las env vars desde el Conf central (mismo patrón que
// casdoor.Admin con CASDOOR_CLIENT_ID/SECRET).
func NewAdmin(env environment.Conf) *Admin {
	return &Admin{
		baseURL:  strings.TrimRight(env.OPENOBSERVE_ENDPOINT_INTERNAL, "/"),
		user:     env.ZO_ROOT_USER_EMAIL,
		password: env.ZO_ROOT_USER_PASSWORD,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateOrganization crea un org en OO con el `name` indicado.
//
// OO devuelve un `identifier` random (ej. "3DQXGgMFPCN..."), que es
// el ID que tenemos que usar en URLs. Lo devolvemos al caller para
// que lo persista en `tenants.openobserve_org_id` y arme el iframe URL.
//
// Si el name ya existe, retorna ErrAlreadyExists.
func (a *Admin) CreateOrganization(ctx context.Context, name string) (identifier string, err error) {
	body, _ := json.Marshal(map[string]string{"name": name})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/api/organizations", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.user, a.password)

	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openobserve add-organization: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("%w: %s", ErrUnauthorized, string(raw))
	}

	// Errores 4xx con mensaje JSON:
	if resp.StatusCode >= 400 {
		var errBody struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &errBody)
		if strings.Contains(strings.ToLower(errBody.Message), "already exists") {
			return "", fmt.Errorf("%w: %s", ErrAlreadyExists, errBody.Message)
		}
		return "", fmt.Errorf("openobserve %d: %s", resp.StatusCode, errBody.Message)
	}

	var ok struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return "", fmt.Errorf("openobserve response unmarshal: %w; raw: %s", err, string(raw))
	}
	if ok.Identifier == "" {
		return "", fmt.Errorf("openobserve created org but no identifier returned: %s", string(raw))
	}
	return ok.Identifier, nil
}

// CreateUser crea un user en una org específica de OO con la role
// indicada ("admin" | "member" | "root"). Idempotente conceptualmente:
// si el email ya existe en la org, OO devuelve OK con "already exists"
// (lo tratamos como ErrAlreadyExists).
//
// La password se manda en plaintext sobre TLS (interno docker network).
// OO la hashea internamente antes de persistir.
func (a *Admin) CreateUser(ctx context.Context, orgID, email, password, role string) error {
	body, _ := json.Marshal(map[string]string{
		"email":      email,
		"password":   password,
		"role":       role,
		"first_name": email, // OO requiere first_name no vacío
		"last_name":  "-",
	})

	url := a.baseURL + "/api/" + orgID + "/users"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.user, a.password)

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("openobserve add-user: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", ErrUnauthorized, string(raw))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("openobserve add-user %d: %s", resp.StatusCode, string(raw))
	}

	var ok struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &ok); err == nil {
		if strings.Contains(strings.ToLower(ok.Message), "already exists") ||
			strings.Contains(strings.ToLower(ok.Message), "already in this organization") {
			return ErrAlreadyExists
		}
	}
	return nil
}

// SlugToOrgName convierte un tenant slug ("acme-corp") a un nombre
// válido para OpenObserve ("acme_corp"). OO rechaza guiones.
func SlugToOrgName(slug string) string {
	return strings.ReplaceAll(slug, "-", "_")
}

// GeneratePassword devuelve un random hex de 32 chars (128 bits).
// Usable como password de un user (entropy más que suficiente, sin
// caracteres problemáticos).
func GeneratePassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
