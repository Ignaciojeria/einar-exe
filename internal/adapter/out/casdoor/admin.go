// Package casdoor encapsula llamadas server-to-server a la admin API
// de Casdoor. Toda interacción con Casdoor que NO sea OIDC vive aquí.
//
// Auth: usamos `clientId:clientSecret` de la application registrada
// (campo `isAdmin: true` en init_data.json.tpl) como Basic Auth.
// Esto NO es lo mismo que `CASDOOR_ADMIN_USERNAME`/PASSWORD, que son
// credenciales del user humano `admin` para la UI.
package casdoor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewAdmin)

// Errores que el caller (handlers) chequea con errors.Is.
var (
	ErrAlreadyExists = errors.New("casdoor: object already exists")
	ErrUnauthorized  = errors.New("casdoor: client credentials rejected")
)

// Admin es el cliente HTTP a la admin API. Lleva el HTTP client + las
// credenciales pre-cargadas. Mantén una sola instancia por proceso.
type Admin struct {
	baseURL      string // ej. http://casdoor:8000 (interno docker)
	clientID     string
	clientSecret string
	http         *http.Client
}

// NewAdmin construye el cliente. Usa el endpoint INTERNAL para evitar
// que las llamadas server-to-server salgan por el edge de exe.dev.
func NewAdmin(env environment.Conf) *Admin {
	return &Admin{
		baseURL:      env.CASDOOR_ENDPOINT_INTERNAL,
		clientID:     env.CASDOOR_CLIENT_ID,
		clientSecret: env.CASDOOR_CLIENT_SECRET,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// apiResponse es el wrapper estándar de Casdoor: { status, msg, data, ... }.
type apiResponse struct {
	Status string          `json:"status"` // "ok" | "error"
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

// CreateOrganization crea un org en Casdoor. Idempotente desde la perspectiva
// del caller: si el org ya existe (ErrAlreadyExists), el caller decide si
// continuar (común si el flujo de signup se reintenta).
//
// `name` debe coincidir con el slug del tenant en nuestra DB para que la
// trazabilidad sea trivial. `displayName` es el nombre humano-friendly.
func (a *Admin) CreateOrganization(ctx context.Context, name, displayName string) error {
	body := map[string]any{
		"owner":           "admin",
		"name":            name,
		"displayName":     displayName,
		"websiteUrl":      "", // se setea cuando el tenant tenga su propia URL
		"passwordType":    "plain",
		"passwordOptions": []string{},
		"countryCodes":    []string{"US"},
	}
	return a.postObject(ctx, "/api/add-organization", body)
}

// DeleteOrganization elimina un org. Lo usamos para compensar fallos
// transaccionales (signup creó tenant + org pero falló el commit).
func (a *Admin) DeleteOrganization(ctx context.Context, name string) error {
	body := map[string]any{
		"owner": "admin",
		"name":  name,
	}
	return a.postObject(ctx, "/api/delete-organization", body)
}

// postObject envía un POST con basic auth, JSON body, y mapea el wrapper
// de Casdoor a errores tipados.
func (a *Admin) postObject(ctx context.Context, path string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.clientID, a.clientSecret)

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("casdoor %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", ErrUnauthorized, string(raw))
	}

	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return fmt.Errorf("casdoor %s: bad response (%d): %s",
			path, resp.StatusCode, string(raw))
	}

	if ar.Status == "ok" {
		return nil
	}

	// Casdoor devuelve `status: "error"` con mensaje semánticamente significativo.
	// "already exists" lo subimos como ErrAlreadyExists para que el caller pueda
	// decidir si rollback o continuar.
	if containsAny(ar.Msg, "already exist", "duplicate") {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, ar.Msg)
	}

	return fmt.Errorf("casdoor %s error: %s", path, ar.Msg)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) && bytesContainsFold([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}

// bytesContainsFold es contains case-insensitive sin importar `strings`.
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
