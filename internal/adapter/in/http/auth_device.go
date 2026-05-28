package http

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var _ = ioc.Register(authDeviceHandler)

// ── In-memory store para device sessions ─────────────────────────────────────
// En producción se movería a Redis/Postgres. Para un solo nodo es OK.

const (
	deviceCodeTTL           = 15 * time.Minute
	deviceMaxActiveSessions = 100
	deviceMinPollInterval   = 5 * time.Second
)

type deviceSession struct {
	DeviceCode   string // opaque, lo usa el CLI para polling
	SessionID    string // UUID en la URL que abre el usuario
	ExpiresAt    time.Time
	IDToken      string
	RefreshToken string
	Authorized   bool
	LastPoll     time.Time
}

var (
	deviceMu      sync.Mutex
	devicesByID   = map[string]*deviceSession{} // session_id (UUID) → session
	devicesByCode = map[string]*deviceSession{} // device_code → session

	// Rate limit para POST /api/auth/device/code
	deviceCodeRateMu   sync.Mutex
	deviceCodeRateByIP = map[string]time.Time{}
)

func cleanExpiredDeviceSessions() {
	now := time.Now()
	for k, s := range devicesByID {
		if now.After(s.ExpiresAt) {
			delete(devicesByID, k)
		}
	}
	for k, s := range devicesByCode {
		if now.After(s.ExpiresAt) {
			delete(devicesByCode, k)
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

// ── Request/Response types ───────────────────────────────────────────────────

type DeviceCodeRequest struct {
	ClientID string `json:"client_id"`
}

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	VerificationURI string `json:"verification_uri"`
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
	// ── POST /api/auth/device/code ──────────────────────────────────────
	// CLI llama esto. Recibe un device_code (para polling) y una URL
	// con UUID embebido que el usuario abre en cualquier browser.
	fuego.Post(s, "/api/auth/device/code",
		func(c fuego.ContextWithBody[DeviceCodeRequest]) (DeviceCodeResponse, error) {
			body, err := c.Body()
			if err != nil {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 400, Title: "bad request"}
			}
			if body.ClientID != env.CASDOOR_CLIENT_ID {
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 401, Title: "invalid client_id"}
			}

			// Rate limit: 1 req / 10s por IP
			ip := clientIP(c.Request())
			deviceCodeRateMu.Lock()
			if last, ok := deviceCodeRateByIP[ip]; ok && time.Since(last) < 10*time.Second {
				deviceCodeRateMu.Unlock()
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 429, Title: "slow_down"}
			}
			deviceCodeRateByIP[ip] = time.Now()
			deviceCodeRateMu.Unlock()

			// Max active sessions
			deviceMu.Lock()
			cleanExpiredDeviceSessions()
			if len(devicesByCode) >= deviceMaxActiveSessions {
				deviceMu.Unlock()
				return DeviceCodeResponse{}, fuego.HTTPError{Status: 503, Title: "too many active sessions"}
			}
			deviceMu.Unlock()

			sessionID := uuid.New().String()
			deviceCode := uuid.New().String()

			sess := &deviceSession{
				DeviceCode: deviceCode,
				SessionID:  sessionID,
				ExpiresAt:  time.Now().Add(deviceCodeTTL),
			}

			deviceMu.Lock()
			devicesByID[sessionID] = sess
			devicesByCode[deviceCode] = sess
			deviceMu.Unlock()

			baseURL := strings.TrimRight(env.APP_PUBLIC_URL, "/")

			return DeviceCodeResponse{
				DeviceCode:      deviceCode,
				VerificationURI: fmt.Sprintf("%s/auth/device/%s", baseURL, sessionID),
				ExpiresIn:       900,
				Interval:        5,
			}, nil
		})

	// ── POST /api/auth/device/token ─────────────────────────────────────
	// CLI hace polling cada 5s con el device_code.
	fuego.Post(s, "/api/auth/device/token",
		func(c fuego.ContextWithBody[DeviceTokenRequest]) (DeviceTokenResponse, error) {
			body, err := c.Body()
			if err != nil {
				return DeviceTokenResponse{}, fuego.HTTPError{Status: 400, Title: "bad request"}
			}

			deviceMu.Lock()
			sess, ok := devicesByCode[body.DeviceCode]
			if ok {
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

			// Authorized — entregar tokens y limpiar (single-use).
			idToken := sess.IDToken
			refreshToken := sess.RefreshToken

			deviceMu.Lock()
			delete(devicesByID, sess.SessionID)
			delete(devicesByCode, sess.DeviceCode)
			deviceMu.Unlock()

			return DeviceTokenResponse{
				AccessToken:  idToken,
				IDToken:      idToken,
				RefreshToken: refreshToken,
				TokenType:    "Bearer",
				ExpiresIn:    604800,
			}, nil
		})

	// ── GET /auth/device/{id} ───────────────────────────────────────────
	// El usuario abre esta URL directamente desde el CLI.
	// Si el UUID es válido → redirige directo a OAuth (sin formulario).
	fuego.GetStd(s, "/auth/device/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		deviceMu.Lock()
		sess, ok := devicesByID[sessionID]
		deviceMu.Unlock()

		if !ok || time.Now().After(sess.ExpiresAt) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(404)
			fmt.Fprint(w, deviceErrorHTML("Link expirado o inválido. Volvé a ejecutar el comando en tu CLI."))
			return
		}

		// Guardar session ID en cookie para recuperarlo después del OAuth
		http.SetCookie(w, &http.Cookie{
			Name:     "einar_device_session",
			Value:    sessionID,
			Path:     "/",
			MaxAge:   900,
			HttpOnly: true,
			Secure:   secureCookies(env),
			SameSite: http.SameSiteLaxMode,
		})

		// Redirigir directo al login OAuth
		http.Redirect(w, r, "/auth/login?return=/auth/device/complete", http.StatusFound)
	}))

	// ── GET /auth/device/complete ───────────────────────────────────────
	// Después del OAuth callback, el usuario aterriza aquí.
	// Leemos la sesión OAuth (cookies) y la ligamos al device session.
	fuego.GetStd(s, "/auth/device/complete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Recuperar session ID de la cookie
		sc, err := r.Cookie("einar_device_session")
		if err != nil || sc.Value == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(400)
			fmt.Fprint(w, deviceErrorHTML("Sesión de device flow perdida. Volvé a intentar."))
			return
		}
		sessionID := sc.Value

		// Limpiar cookie
		http.SetCookie(w, &http.Cookie{Name: "einar_device_session", Value: "", Path: "/", MaxAge: -1})

		// 2. Leer tokens de la sesión OAuth (seteados por /auth/callback)
		sessionCookie, err := r.Cookie(cookieSession)
		if err != nil || sessionCookie.Value == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(401)
			fmt.Fprint(w, deviceErrorHTML("No se pudo obtener la sesión. ¿Completaste el login?"))
			return
		}

		refreshToken := ""
		if rc, err := r.Cookie(cookieRefresh); err == nil {
			refreshToken = rc.Value
		}

		// 3. Ligar tokens al device session
		deviceMu.Lock()
		sess, ok := devicesByID[sessionID]
		if ok && !time.Now().After(sess.ExpiresAt) {
			sess.IDToken = sessionCookie.Value
			sess.RefreshToken = refreshToken
			sess.Authorized = true
		}
		deviceMu.Unlock()

		if !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(400)
			fmt.Fprint(w, deviceErrorHTML("Link expirado. Volvé a intentar desde tu CLI."))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprint(w, deviceSuccessHTML())
	}))
}

// ── HTML ─────────────────────────────────────────────────────────────────────

func deviceErrorHTML(msg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Error</title>
<style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f5f5f5}.card{background:#fff;border-radius:12px;padding:2.5rem;box-shadow:0 4px 24px rgba(0,0,0,.1);max-width:400px;text-align:center}.err{color:#dc2626;font-size:1.1rem}</style>
</head><body><div class="card"><p class="err">❌ %s</p></div></body></html>`, msg)
}

func deviceSuccessHTML() string {
	return `<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>CLI Autorizado</title>
<style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f5f5f5}.card{background:#fff;border-radius:12px;padding:2.5rem;box-shadow:0 4px 24px rgba(0,0,0,.1);max-width:400px;text-align:center}.ok{color:#16a34a;font-size:1.3rem;font-weight:700}p{color:#666;margin-top:1rem}</style>
</head><body><div class="card"><p class="ok">✅ CLI Autorizado</p><p>Podés cerrar esta pestaña y volver a tu terminal.</p></div></body></html>`
}
