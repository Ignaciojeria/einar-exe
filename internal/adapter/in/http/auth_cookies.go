package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"einar-exe/internal/shared/environment"
)

// Nombres de cookies. Centralizados para que login/callback/logout/middleware
// los compartan sin riesgo de typos.
const (
	cookieSession = "einar_session"      // id_token JWT (sesión efímera, ~15min)
	cookieRefresh = "einar_refresh"      // refresh_token (largo, ~30d)
	cookieState   = "einar_oauth_state"  // CSRF: random opaque, vida = 1 transacción OAuth
	cookieReturn  = "einar_oauth_return" // ruta a la que redirigir tras login
)

// secureCookies devuelve true si la app está sirviendo por HTTPS público,
// lo que obliga a Secure=true. En dev (`http://localhost`) lo dejamos false
// para que la cookie funcione sin TLS local.
func secureCookies(env environment.Conf) bool {
	return env.APP_ENV != "development" || (len(env.APP_PUBLIC_URL) >= 8 && env.APP_PUBLIC_URL[:8] == "https://")
}

// newStateToken genera un opaque random de 32 bytes (256 bits) hex-encoded.
// Usado como `state` en OAuth para mitigar CSRF en el callback.
func newStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// setSessionCookie guarda el id_token en una cookie HttpOnly.
// `maxAgeSec` debería alinearse con el `exp` del JWT (Casdoor: 168h por
// defecto en `init_data.json.tpl`).
func setSessionCookie(w http.ResponseWriter, env environment.Conf, idToken string, maxAgeSec int) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSession,
		Value:    idToken,
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secureCookies(env),
		SameSite: http.SameSiteLaxMode,
	})
}

// setRefreshCookie guarda el refresh_token. Path=/ (no path-restricted)
// porque el middleware de auto-refresh corre en /api/* y necesita leerlo.
// La protección real es HttpOnly+Secure+SameSite, no el path scope.
//
// `maxAgeSec` debería igualar el TTL del refresh_token en Casdoor
// (default 30d; ajustar en init_data.json.tpl si se cambia).
func setRefreshCookie(w http.ResponseWriter, env environment.Conf, refreshToken string, maxAgeSec int) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefresh,
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secureCookies(env),
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie envía un Set-Cookie con MaxAge negativo para borrar.
func clearCookie(w http.ResponseWriter, name string, env environment.Conf) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secureCookies(env),
		SameSite: http.SameSiteLaxMode,
	})
}

// setShortCookie guarda valores temporales (state, return URL) por 10 min.
func setShortCookie(w http.ResponseWriter, name, value string, env environment.Conf) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  time.Now().Add(10 * time.Minute),
		HttpOnly: true,
		Secure:   secureCookies(env),
		SameSite: http.SameSiteLaxMode,
	})
}
