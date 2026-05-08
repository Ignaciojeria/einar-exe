// Package web embebe el bundle del SPA generado por Vite.
//
// El folder `dist/` lo crea `cd web && npm run build` (config en
// web/vite.config.ts → outDir = ../internal/web/dist).
//
// 12-factor V (build/release/run): el binario Go se entrega
// auto-contenido. Sin folders externos al deploy.
//
// Si compilas Go sin haber generado el bundle antes, go:embed falla
// con un mensaje claro mencionando que falta `dist/`. Ese fallo es
// intencional: prefiere fallar al build que arrancar con un SPA en
// blanco.
package web

import "embed"

//go:embed all:dist
var FS embed.FS
