// Package postgres expone el `*pgxpool.Pool` (un singleton via IoC) y
// las implementaciones concretas de los repositorios definidos en
// `internal/domain`.
//
// 12-factor IV: la conexión se construye desde `DATABASE_URL`. Cambiar
// de Postgres local a uno gestionado debe ser solo cambiar la env var.
package postgres

import (
	"context"
	"fmt"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = ioc.Register(NewPool)

// NewPool construye y prueba un pool de conexiones a Postgres.
//
// Decisiones:
//   - Pool default de pgx (sane defaults: ~maxconns CPU*4, idle 30min).
//     Si necesitas tunear, usa `pool.Config()`.
//   - Ping con timeout corto al arrancar: si la DB no responde en 5s,
//     la app falla rápido en vez de quedar zombie esperando conexiones.
//   - Ningún `defer pool.Close()` aquí: el pool vive lo mismo que el
//     proceso. El cleanup en shutdown se hace via IoC RegisterAtEnd
//     si alguien lo necesita; por ahora pgx maneja todo bien al exit.
func NewPool(env environment.Conf) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), env.DATABASE_URL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	return pool, nil
}
