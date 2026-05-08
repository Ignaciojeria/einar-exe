package http

import (
	"net/http"

	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(signupPageHandler)

// signupPageHandler — GET /signup
//
// Página HTML mínima que pide slug + displayName y postea a /api/signup.
// Es plomería temporal hasta que el shell SPA (Fase 3) tenga onboarding.
//
// Decisiones:
//   - Same-origin POST con `credentials: 'include'` → la cookie HttpOnly
//     viaja sola, no necesitamos exponer el token al JS.
//   - Cero deps: HTML+JS inline. Cuando exista el shell, se borra.
//   - Requiere sesión (registrada bajo el grupo /api... NO, espera, vive
//     en /signup que NO está bajo /api). El handler chequea cookie
//     manualmente y redirige a /auth/login si no hay sesión.
func signupPageHandler(s *fuego.Server) {
	fuego.GetStd(s, "/signup", func(w http.ResponseWriter, r *http.Request) {
		// Auth manual: si no hay cookie, redirige a login con return.
		if _, err := r.Cookie("einar_session"); err != nil {
			http.Redirect(w, r, "/auth/login?return=/signup", http.StatusFound)
			return
		}
		// Si el user YA tiene tenant, no debería ver esta página.
		// Lo verificará el frontend leyendo /api/session; el backend
		// no bloquea (deja que /api/signup devuelva 409 si reintenta).

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(signupHTML))
	})

	// Middleware del lado del usuario maneja el contexto, pero esta
	// función `withMiddleware` no se usa: dejamos el bloqueo client-side
	// + el chequeo de cookie inline. El POST /api/signup sí está
	// protegido server-side.
	_ = middleware.UserFromContext
}

const signupHTML = `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Crear workspace · einar</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 480px; margin: 4rem auto; padding: 0 1rem; }
  h1 { font-size: 1.4rem; margin-bottom: .5rem; }
  p.muted { color: #666; margin-top: 0; }
  label { display: block; margin: 1rem 0 .25rem; font-weight: 600; }
  input { width: 100%; padding: .5rem; font-size: 1rem; box-sizing: border-box;
          border: 1px solid #ccc; border-radius: 4px; }
  button { margin-top: 1.5rem; padding: .6rem 1rem; font-size: 1rem; cursor: pointer;
           background: #111; color: #fff; border: 0; border-radius: 4px; }
  button:disabled { background: #888; cursor: wait; }
  .error { margin-top: 1rem; padding: .75rem; background: #fee; color: #c00;
           border: 1px solid #fcc; border-radius: 4px; }
  .hint { font-size: .85rem; color: #888; margin-top: .25rem; }
</style>
</head>
<body>
  <h1>Crear tu workspace</h1>
  <p class="muted">Es el último paso. Vas a poder entrar después en
  <code>einar.exe.xyz/t/&lt;slug&gt;/</code></p>

  <form id="f">
    <label for="slug">Slug del workspace</label>
    <input id="slug" name="slug" required pattern="[a-z0-9][a-z0-9-]{1,30}[a-z0-9]"
           autocomplete="off" placeholder="acme">
    <div class="hint">Lowercase, números y guiones. 3-32 caracteres.</div>

    <label for="displayName">Nombre visible</label>
    <input id="displayName" name="displayName" required maxlength="80"
           placeholder="Acme Corp">

    <button type="submit" id="btn">Crear workspace</button>
  </form>
  <div id="err" class="error" style="display:none"></div>

<script>
const f = document.getElementById('f');
const btn = document.getElementById('btn');
const err = document.getElementById('err');

f.addEventListener('submit', async (e) => {
  e.preventDefault();
  err.style.display = 'none';
  btn.disabled = true; btn.textContent = 'Creando...';
  try {
    const r = await fetch('/api/signup', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify({
        slug: f.slug.value.trim().toLowerCase(),
        displayName: f.displayName.value.trim(),
      }),
    });
    const j = await r.json();
    if (!r.ok) throw new Error(j.title || j.detail || ('HTTP '+r.status));
    window.location = j.redirectTo || '/';
  } catch (ex) {
    err.textContent = ex.message;
    err.style.display = 'block';
    btn.disabled = false; btn.textContent = 'Crear workspace';
  }
});
</script>
</body>
</html>
`
