// Backend de ejemplo para una embedded app de einar.
//
// Mantiene 0 estado, valida JWTs venidos del SDK del shell, y expone
// un endpoint /api/secret que devuelve datos personalizados según el
// claim `tenant_slug` del JWT.
//
// Run: go run .
// Listens: :3001
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	issuer := envOr("EINAR_ISSUER", "https://einar.exe.xyz")
	jwksURI := envOr("EINAR_JWKS_URI", issuer+"/.well-known/einar/jwks")

	auth, err := NewAuth(issuer, jwksURI)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}

	mux := http.NewServeMux()

	// Endpoint protegido por el middleware JWT.
	mux.Handle("GET /api/secret", auth.Middleware(http.HandlerFunc(secretHandler)))

	// Health.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	// CORS minimal — los browsers van a hacer preflight cuando el iframe
	// fetchee tu backend desde otro origin.
	handler := corsMiddleware(mux)

	addr := ":" + envOr("PORT", "3001")
	log.Printf("listening on %s (issuer=%s)", addr, issuer)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func secretHandler(w http.ResponseWriter, r *http.Request) {
	c := AuthFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"msg":         fmt.Sprintf("hello %s, you are %s in tenant %s", c.Email, c.Role, c.TenantSlug),
		"sub":         c.Subject,
		"tenant_id":   c.TenantID,
		"tenant_slug": c.TenantSlug,
		"role":        c.Role,
		"exp":         c.ExpiresAt.Unix(),
	})
}

// corsMiddleware permite que cualquier origin embebido en einar
// llame al backend. En producción restringilo a tu UI.
func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
