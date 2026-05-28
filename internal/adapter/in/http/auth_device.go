package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authDeviceHandler)

// ── In-memory store para device codes ────────────────────────────────
// En producción se movería a Redis/Postgres. Para un solo nodo es OK.

const (
	deviceCodeTTL          = 15 * time.Minute
	deviceMaxActiveSessions = 100   // max sesiones simultáneas (previene memory exhaustion)
	deviceMaxFailedAttempts = 5     // max intentos fallidos por IP antes de lockout
	deviceFailedWindow     = 10 * time.Minute
	deviceMinPollInterval  = 5 * time.Second
)

type deviceSession struct {
	DeviceCode   string
	UserCode     string
	ExpiresAt    time.Time
	Interval     int // polling interval en segundos
	IDToken      string
	RefreshToken string
	Authorized   bool
	LastPoll     time.Time // para throttle de polling
}

type failedAttempt struct {
	Count    int
	FirstAt  time.Time
}

var (
	deviceMu       sync.Mutex
	devicesByUser   = map[string]*deviceSession{} // user_code → session
	devicesByDevice = map[string]*deviceSession{} // device_code → session

	// Rate limiting: IP → failed attempts para /auth/device/confirm
	deviceFailedMu    sync.Mutex
	deviceFailedByIP  = map[string]*failedAttempt{}

	// Rate limiting: IP → last request para /api/auth/device/code
	deviceCodeRateMu  sync.Mutex
	deviceCodeRateByIP = map[string]time.Time{}
)

func cleanExpiredDeviceSessions() {
	now := time.Now()
	for k, s := range devicesByUser {
		if now.After(s.ExpiresAt) {
			delete(devicesByUser, k)
		}
	}
	for k, s := range devicesByDevice {
		if now.After(s.ExpiresAt) {
			delete(devicesByDevice, k)
		}
	}
}

func cleanExpiredFailedAttempts() {
	now := time.Now()
	for k, v := range deviceFailedByIP {
		if now.Sub(v.FirstAt) > deviceFailedWindow {
			delete(deviceFailedByIP, k)
		}
	}
}

// isIPBlocked checks if an IP has too many failed code confirmations.
func isIPBlocked(ip string) bool {
	deviceFailedMu.Lock()
	defer deviceFailedMu.Unlock()
	cleanExpiredFailedAttempts()
	fa, ok := deviceFailedByIP[ip]
	if !ok {
		return false
	}
	return fa.Count >= deviceMaxFailedAttempts
}

// recordFailedAttempt increments failed attempts for an IP.
func recordFailedAttempt(ip string) {
	deviceFailedMu.Lock()
	defer deviceFailedMu.Unlock()
	fa, ok := deviceFailedByIP[ip]
	if !ok {
		deviceFailedByIP[ip] = &failedAttempt{Count: 1, FirstAt: time.Now()}
		return
	}
	fa.Count++
}

// clearFailedAttempts resets on successful confirmation.
func clearFailedAttempts(ip string) {
	deviceFailedMu.Lock()
	defer deviceFailedMu.Unlock()
	delete(deviceFailedByIP, ip)
}

// clientIP extracts the real client IP from the request.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

// generateUserCode genera un código legible tipo "ABCD-EFGH" (8 chars alfanum).
func generateUserCode() (string, error) {
	// Solo consonantes+dígitos para evitar palabras ofensivas accidentales
	const alphabet = "BCDFGHJKLMNPQRSTVWXYZ2345679"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

func generateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ── Handlers ─────────────────────────────────────────────────────────

type DeviceCodeRequest struct {
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
}

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type DeviceTokenRequest struct {
	GrantType  string `json:"grant_type"`
	DeviceCode string `json:"device_code"`
	ClientID   string `json:"client_id"`
}

type DeviceTokenResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
}

func authDeviceHandler(s *fuego.Server, env environment.Conf) {
	// ── POST /api/auth/device/code ──────────────────────────────────
	// CLI llama esto para obtener un user_code que mostrar al usuario.
	fuego.Post(s, "/api/auth/device/code",
		func(c fuego.ContextWithBody[DeviceCodeRequest]) (DeviceCodeResponse, error) {
			body, err := c.Body()
			if err != nil {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 400, Title: "bad request"}
			}
			if body.ClientID != env.CASDOOR_CLIENT_ID {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 401, Title: "invalid client_id"}
			}

			// Rate limit: max 1 request per 10s per IP
			ip := clientIP(c.Request())
			deviceCodeRateMu.Lock()
			last, exists := deviceCodeRateByIP[ip]
			if exists && time.Since(last) < 10*time.Second {
				deviceCodeRateMu.Unlock()
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 429, Title: "slow_down", Detail: "wait 10 seconds between requests"}
			}
			deviceCodeRateByIP[ip] = time.Now()
			deviceCodeRateMu.Unlock()

			// Max active sessions globally
			deviceMu.Lock()
			cleanExpiredDeviceSessions()
			if len(devicesByDevice) >= deviceMaxActiveSessions {
				deviceMu.Unlock()
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 503, Title: "too many active device sessions"}
			}
			deviceMu.Unlock()

			userCode, err := generateUserCode()
			if err != nil {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 500, Title: "code generation failed"}
			}
			deviceCode, err := generateDeviceCode()
			if err != nil {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 500, Title: "code generation failed"}
			}

			sess := &deviceSession{
				DeviceCode: deviceCode,
				UserCode:   userCode,
				ExpiresAt:  time.Now().Add(15 * time.Minute),
				Interval:   5,
			}

			deviceMu.Lock()
			cleanExpiredDeviceSessions()
			devicesByUser[userCode] = sess
			devicesByDevice[deviceCode] = sess
			deviceMu.Unlock()

			baseURL := strings.TrimRight(env.APP_PUBLIC_URL, "/")
			verificationURI := baseURL + "/auth/device"

			return DeviceCodeResponse{
				DeviceCode:              deviceCode,
				UserCode:                userCode,
				VerificationURI:         verificationURI,
				VerificationURIComplete: verificationURI + "?code=" + userCode,
				ExpiresIn:               900, // 15 min
				Interval:                5,
			}, nil
		})

	// ── POST /api/auth/device/token ─────────────────────────────────
	// CLI hace polling aquí hasta que el usuario complete el login.
	fuego.Post(s, "/api/auth/device/token",
		func(c fuego.ContextWithBody[DeviceTokenRequest]) (DeviceTokenResponse, error) {
			body, err := c.Body()
			if err != nil {
				return DeviceTokenResponse{}, fuego.HTTPError{Status: 400, Title: "bad request"}
			}

			deviceMu.Lock()
			sess, ok := devicesByDevice[body.DeviceCode]
			if ok {
				// Throttle: enforce minimum poll interval
				if time.Since(sess.LastPoll) < deviceMinPollInterval {
					deviceMu.Unlock()
					return DeviceTokenResponse{Error: "slow_down"}, nil
				}
				sess.LastPoll = time.Now()
			}
			deviceMu.Unlock()

			if !ok || time.Now().After(sess.ExpiresAt) {
				return DeviceTokenResponse{Error: "expired_token"}, nil
			}

			if !sess.Authorized {
				return DeviceTokenResponse{Error: "authorization_pending"}, nil
			}

			// Authorized! Return tokens and clean up.
			idToken := sess.IDToken
			refreshToken := sess.RefreshToken

			deviceMu.Lock()
			delete(devicesByUser, sess.UserCode)
			delete(devicesByDevice, sess.DeviceCode)
			deviceMu.Unlock()

			return DeviceTokenResponse{
				AccessToken:  idToken,
				IDToken:      idToken,
				RefreshToken: refreshToken,
				TokenType:    "Bearer",
				ExpiresIn:    604800, // 7 días (Casdoor default)
			}, nil
		})

	// ── GET /auth/device ────────────────────────────────────────────
	// Página donde el usuario ingresa o confirma el user_code.
	// Si ?code=XXXX-YYYY viene en la URL, pre-rellena.
	fuego.GetStd(s, "/auth/device", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCode := strings.TrimSpace(r.URL.Query().Get("code"))
		html := devicePageHTML(env, userCode)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(html))
	}))

	// ── POST /auth/device/confirm ───────────────────────────────────
	// El formulario HTML envía el user_code aquí. Redirige a Casdoor
	// login con state = device:{user_code} para que el callback sepa
	// que es un device flow.
	fuego.PostStd(s, "/auth/device/confirm", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		userCode := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
		ip := clientIP(r)

		// Brute-force protection
		if isIPBlocked(ip) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(429)
			w.Write([]byte(deviceErrorHTML("Demasiados intentos fallidos. Esperá 10 minutos.")))
			return
		}

		deviceMu.Lock()
		_, exists := devicesByUser[userCode]
		deviceMu.Unlock()

		if !exists {
			recordFailedAttempt(ip)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(400)
			w.Write([]byte(deviceErrorHTML("Código inválido o expirado. Volvé a intentar desde tu CLI.")))
			return
		}
		clearFailedAttempts(ip)

		// Guardamos el user_code en una cookie para recuperarlo en el callback.
		http.SetCookie(w, &http.Cookie{
			Name:     "einar_device_code",
			Value:    userCode,
			Path:     "/",
			MaxAge:   900,
			HttpOnly: true,
			Secure:   secureCookies(env),
			SameSite: http.SameSiteLaxMode,
		})

		// Redirigir a /auth/login con return=/auth/device/complete
		http.Redirect(w, r, "/auth/login?return=/auth/device/complete", http.StatusFound)
	}))

	// ── GET /auth/device/complete ───────────────────────────────────
	// Después de que el usuario se autentica (callback normal seteó
	// cookies de sesión), aterrizan aquí. Leemos la sesión y la
	// ligamos al device_code.
	fuego.GetStd(s, "/auth/device/complete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Leer user_code de la cookie
		codeCookie, err := r.Cookie("einar_device_code")
		if err != nil || codeCookie.Value == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(400)
			w.Write([]byte(deviceErrorHTML("Sesión de device flow perdida. Volvé a intentar.")))
			return
		}
		userCode := codeCookie.Value

		// Limpiar cookie
		http.SetCookie(w, &http.Cookie{
			Name:   "einar_device_code",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		// 2. Leer el id_token de la sesión que el callback normal ya seteó
		sessionCookie, err := r.Cookie(cookieSession)
		if err != nil || sessionCookie.Value == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(401)
			w.Write([]byte(deviceErrorHTML("No se pudo obtener la sesión. ¿Completaste el login?")))
			return
		}

		refreshToken := ""
		if rc, err := r.Cookie(cookieRefresh); err == nil {
			refreshToken = rc.Value
		}

		// 3. Ligar tokens al device session
		deviceMu.Lock()
		sess, ok := devicesByUser[userCode]
		if ok {
			sess.IDToken = sessionCookie.Value
			sess.RefreshToken = refreshToken
			sess.Authorized = true
		}
		deviceMu.Unlock()

		if !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(400)
			w.Write([]byte(deviceErrorHTML("Código expirado. Volvé a intentar desde tu CLI.")))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(deviceSuccessHTML()))
	}))
}

// ── HTML templates ───────────────────────────────────────────────────

func devicePageHTML(env environment.Conf, prefilledCode string) string {
	value := ""
	if prefilledCode != "" {
		value = fmt.Sprintf(` value="%s"`, prefilledCode)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Einar CLI Login</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
  .card { background: white; border-radius: 12px; padding: 2.5rem; box-shadow: 0 4px 24px rgba(0,0,0,0.1); max-width: 400px; width: 100%%; text-align: center; }
  h1 { font-size: 1.5rem; color: #1a1a2e; margin: 0 0 0.5rem; }
  p { color: #666; margin: 0.5rem 0 1.5rem; font-size: 0.95rem; }
  input[type=text] { font-size: 1.8rem; text-align: center; letter-spacing: 0.3em; padding: 0.8rem; border: 2px solid #ddd; border-radius: 8px; width: 200px; font-family: monospace; text-transform: uppercase; }
  input:focus { outline: none; border-color: #7c3aed; }
  button { background: #7c3aed; color: white; border: none; padding: 0.8rem 2rem; font-size: 1rem; border-radius: 8px; cursor: pointer; margin-top: 1rem; width: 100%%; }
  button:hover { background: #6d28d9; }
  .logo { font-size: 2rem; margin-bottom: 1rem; }
</style>
</head>
<body>
<div class="card">
  <div class="logo">⚡</div>
  <h1>Einar CLI Login</h1>
  <p>Ingresá el código que muestra tu terminal</p>
  <form action="/auth/device/confirm" method="POST">
    <input type="text" name="code" maxlength="9" placeholder="ABCD-EFGH" autocomplete="off" autofocus%s>
    <br>
    <button type="submit">Confirmar</button>
  </form>
</div>
</body>
</html>`, value)
}

func deviceErrorHTML(msg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head><meta charset="utf-8"><title>Error</title>
<style>
  body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
  .card { background: white; border-radius: 12px; padding: 2.5rem; box-shadow: 0 4px 24px rgba(0,0,0,0.1); max-width: 400px; text-align: center; }
  .err { color: #dc2626; font-size: 1.1rem; }
</style>
</head>
<body><div class="card"><p class="err">❌ %s</p></div></body>
</html>`, msg)
}

func deviceSuccessHTML() string {
	return `<!DOCTYPE html>
<html lang="es">
<head><meta charset="utf-8"><title>CLI Autorizado</title>
<style>
  body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
  .card { background: white; border-radius: 12px; padding: 2.5rem; box-shadow: 0 4px 24px rgba(0,0,0,0.1); max-width: 400px; text-align: center; }
  .ok { color: #16a34a; font-size: 1.3rem; font-weight: bold; }
  p { color: #666; margin-top: 1rem; }
</style>
</head>
<body><div class="card"><p class="ok">✅ CLI Autorizado</p><p>Podés cerrar esta pestaña y volver a tu terminal.</p></div></body>
</html>`
}
