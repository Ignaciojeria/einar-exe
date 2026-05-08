package http

import (
	"fmt"
	"html"
	"net/http"

	"einar-exe/internal/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(tenantPageHandler)

// tenantPageHandler — GET /t/{slug}/{rest...}
//
// Landing temporal del workspace. En Fase 3 esto se reemplaza por el
// shell SPA de React que se monta en `/t/{slug}/...` y maneja routing
// client-side (sidenav, embedded apps, etc.).
//
// Por ahora solo demuestra que:
//   - El middleware de auth funciona (sin sesión → 401).
//   - El middleware de tenant funciona (slug inválido → 404 / no es tu
//     tenant → 403 / OK → renderiza).
//   - Los datos llegan al handler via context.
//
// Routing patterns Go 1.22+:
//   - "/t/{slug}/"      cubre /t/acme/ exactamente
//   - "/t/{slug}/{rest...}" cubre /t/acme/cualquier/cosa
//
// Registramos AMBOS para que tanto la landing como sub-rutas funcionen.
func tenantPageHandler(s *fuego.Server, auth *middleware.Auth, tenant *middleware.Tenant) {
	mws := []func(http.Handler) http.Handler{
		auth.Require(),
		tenant.Require(),
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		t := middleware.TenantFromContext(r.Context())
		u := middleware.UserFromContext(r.Context())

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, tenantHomeHTML,
			html.EscapeString(t.DisplayName),               // <title>
			html.EscapeString(t.DisplayName),               // <h2> sidenav
			html.EscapeString(t.Slug),                      // /t/{slug}
			html.EscapeString(u.Email),                     // Email
			html.EscapeString(u.DisplayName),               // Nombre
			html.EscapeString(stringPtrOrEmpty(t.CasdoorOrg)), // Casdoor org
		)
	}

	// Solo registramos el landing exacto. Sub-rutas ("/t/acme/dashboard")
	// las resolverá React Router client-side cuando exista el SPA (Fase 3).
	fuego.GetStd(s, "/t/{slug}/", handler, fuego.OptionMiddleware(mws...))
}

func stringPtrOrEmpty(s *string) string {
	if s == nil {
		return "(no aprovisionado)"
	}
	return *s
}

const tenantHomeHTML = `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s · einar</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; margin: 0; background: #fafafa; }
  .layout { display: grid; grid-template-columns: 240px 1fr; min-height: 100vh; }
  .sidenav {
    background: #1a1a1a; color: #eee; padding: 1.5rem 1rem;
  }
  .sidenav h2 { margin: 0 0 1.5rem; font-size: 1.1rem; }
  .sidenav .slug { color: #888; font-size: .85rem; font-family: monospace; }
  .sidenav nav a {
    display: block; color: #ddd; text-decoration: none;
    padding: .5rem .75rem; border-radius: 4px; margin: .25rem 0;
  }
  .sidenav nav a:hover { background: #333; }
  .sidenav nav .placeholder { color: #666; font-style: italic; padding: .5rem .75rem; }
  .sidenav .footer {
    position: absolute; bottom: 1rem; left: 1rem; right: 1rem;
    font-size: .8rem; color: #888; border-top: 1px solid #333; padding-top: 1rem;
    width: 208px;
  }
  .main { padding: 2rem; }
  h1 { margin-top: 0; }
  .meta { color: #666; font-size: .9rem; }
  .card {
    background: #fff; border: 1px solid #e0e0e0; border-radius: 6px;
    padding: 1rem 1.5rem; margin-top: 1rem; max-width: 640px;
  }
  .card h3 { margin-top: 0; font-size: 1rem; color: #333; }
  .card dl { margin: 0; display: grid; grid-template-columns: 140px 1fr; gap: .5rem; }
  .card dt { color: #888; }
  .card code { background: #f4f4f4; padding: 1px 6px; border-radius: 3px; }
  .logout {
    display: inline-block; margin-top: 2rem; padding: .4rem .8rem;
    background: #fff; border: 1px solid #ccc; border-radius: 4px;
    text-decoration: none; color: #333;
  }
  .logout:hover { background: #f0f0f0; }
</style>
</head>
<body>
  <div class="layout">
    <aside class="sidenav">
      <h2>%s</h2>
      <div class="slug">/t/%s</div>
      <nav>
        <span class="placeholder">Embedded apps</span>
        <span class="placeholder">— ninguna registrada —</span>
      </nav>
      <div class="footer">
        Shell temporal · Fase 2.<br>
        El SPA real llega en Fase 3.
      </div>
    </aside>
    <main class="main">
      <h1>Workspace listo 🎉</h1>
      <p class="meta">Esta es la landing temporal de tu tenant. La auth, el resolver de tenant
      y la inyección de contexto están funcionando.</p>

      <div class="card">
        <h3>Tu sesión</h3>
        <dl>
          <dt>Email</dt>      <dd>%s</dd>
          <dt>Nombre</dt>     <dd>%s</dd>
        </dl>
      </div>

      <div class="card">
        <h3>Casdoor org</h3>
        <dl>
          <dt>Nombre</dt>     <dd><code>%s</code></dd>
        </dl>
      </div>

      <a class="logout" href="/auth/logout">Cerrar sesión</a>
    </main>
  </div>
</body>
</html>
`
