package http

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"einar-exe/internal/web"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

// El bundle del SPA vive en `internal/web/dist/`, generado por
// `cd web && npm run build`. internal/web lo expone via go:embed.
//
// Workflow:
//   - dev:  `cd web && npm run dev`  → levanta Vite con HMR proxeando
//                                       /api, /auth, /signup, /t a Caddy.
//           o bien:  `npm run build -- --watch` regenera dist/ al guardar
//           y air recompila Go al detectar el cambio en el archivo embebido.
//   - prod: el Dockerfile correrá `npm run build` antes de `go build`.

var _ = ioc.Register(spaHandler)

// spaHandler sirve el SPA bajo rutas que NO matchea otro handler.
//
// Caddy ya enruta /auth/*, /api/*, /signup → backend Go (handlers
// específicos). Lo demás (/, /assets/*, /t/{slug}/...) llega acá.
//
// Estrategia:
//   - Si el path apunta a un archivo real en web-dist/ (ej. /assets/foo.js)
//     → servimos el archivo con cache largo (Vite hashea los nombres).
//   - Sino → devolvemos index.html (no-cache) y React Router decide.
func spaHandler(s *fuego.Server) {
	dist, err := fs.Sub(web.FS, "dist")
	if err != nil {
		panic("spa: sub-fs dist: " + err.Error())
	}
	files := http.FS(dist)
	server := http.FileServer(files)

	handler := func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Asset real → servir con cache largo.
		if clean != "" {
			if _, err := fs.Stat(dist, clean); err == nil {
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				server.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback → index.html.
		f, err := dist.Open("index.html")
		if err != nil {
			http.Error(w, "spa: index.html missing - run `cd web && npm run build`", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write(readAll(f))
	}

	// Net/http mux no tiene wildcard catch-all. Registramos los
	// patterns que efectivamente sirve el SPA. Caddy se encarga de
	// que solo lleguen acá los paths "del frontend".
	fuego.GetStd(s, "/", handler)
	fuego.GetStd(s, "/assets/", handler)
	fuego.GetStd(s, "/sdk/", handler) // SDK servido al iframe embedded
	fuego.GetStd(s, "/signup", handler)
	fuego.GetStd(s, "/t/{slug}/", handler) // subtree: /t/X/ y /t/X/cualquier/cosa
}

// readAll lee todo el contenido de un fs.File. Se usa para servir
// index.html que no implementa io.Seeker.
func readAll(f fs.File) []byte {
	const chunk = 8 * 1024
	buf := make([]byte, 0, chunk)
	tmp := make([]byte, chunk)
	for {
		n, err := f.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}
