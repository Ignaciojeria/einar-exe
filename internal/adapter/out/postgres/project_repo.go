package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"einar-exe/internal/domain"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = ioc.Register(NewProjectRepo)

type projectRepo struct {
	pool *pgxpool.Pool
	env  environment.Conf
}

func NewProjectRepo(pool *pgxpool.Pool, env environment.Conf) domain.ProjectRepo {
	return &projectRepo{pool: pool, env: env}
}

const projectCols = `id, tenant_id, name, slug, path, subdomain, status, db_name, db_user, db_password, created_at, updated_at`

func scanProject(row pgx.Row) (*domain.Project, error) {
	var p domain.Project
	var dbName, dbUser, dbPassword *string
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Slug, &p.Path, &p.Subdomain, &p.Status,
		&dbName, &dbUser, &dbPassword,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if dbName != nil {
		p.DBName = *dbName
	}
	if dbUser != nil {
		p.DBUser = *dbUser
	}
	if dbPassword != nil {
		p.DBPassword = *dbPassword
	}
	return &p, err
}

func (r *projectRepo) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	name, slug, path, subdomain, status, dbName, dbUser, dbPassword string,
) (*domain.Project, error) {
	// ── 1. Crear role + database aislada en Postgres ────────────────
	// CREATE DATABASE no puede correr dentro de una transacción, así que
	// abrimos una conexión admin separada (superuser del cluster).
	if dbName != "" && dbUser != "" && dbPassword != "" {
		if err := r.provisionProjectDatabase(ctx, dbName, dbUser, dbPassword); err != nil {
			return nil, fmt.Errorf("provision project database: %w", err)
		}
	}

	// ── 2. Insertar el proyecto en la tabla ─────────────────────────
	q := `
		INSERT INTO projects (tenant_id, name, slug, path, subdomain, status, db_name, db_user, db_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + projectCols

	var pDBName, pDBUser, pDBPassword *string
	if dbName != "" {
		pDBName = &dbName
	}
	if dbUser != "" {
		pDBUser = &dbUser
	}
	if dbPassword != "" {
		pDBPassword = &dbPassword
	}

	p, err := scanProject(r.pool.QueryRow(ctx, q, tenantID, name, slug, path, subdomain, status, pDBName, pDBUser, pDBPassword))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgErrCodeUniqueViolation:
				return nil, fmt.Errorf("project slug %q: %w", slug, domain.ErrConflict)
			case pgErrCodeCheckViolation:
				return nil, fmt.Errorf("project invalid arg: %w", domain.ErrInvalidArg)
			}
		}
		return nil, fmt.Errorf("insert project: %w", err)
	}

	return p, nil
}

// provisionProjectDatabase crea un role y una database dedicada para el
// proyecto. Usa una conexión directa como superuser (postgres) porque:
//   - CREATE ROLE requiere CREATEROLE o superuser
//   - CREATE DATABASE no puede correr dentro de una transacción
func (r *projectRepo) provisionProjectDatabase(ctx context.Context, dbName, dbUser, dbPassword string) error {
	// Construir DSN admin a partir del DATABASE_URL del pool, pero con
	// user=postgres y database=postgres (la admin DB que siempre existe).
	adminDSN, err := r.buildAdminDSN()
	if err != nil {
		return fmt.Errorf("build admin dsn: %w", err)
	}

	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect as admin: %w", err)
	}
	defer conn.Close(ctx)

	// Crear el role (idempotente: si ya existe, no falla)
	// No podemos usar $1 para identifiers, así que sanitizamos manualmente.
	safeUser := pgx.Identifier{dbUser}.Sanitize()
	safeDB := pgx.Identifier{dbName}.Sanitize()

	// CREATE ROLE ... LOGIN PASSWORD '...'
	_, err = conn.Exec(ctx, fmt.Sprintf(
		"DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s) THEN CREATE ROLE %s LOGIN PASSWORD %s; END IF; END $$",
		quoteLiteral(dbUser), safeUser, quoteLiteral(dbPassword),
	))
	if err != nil {
		return fmt.Errorf("create role %s: %w", dbUser, err)
	}

	// CREATE DATABASE (no puede ir dentro de un DO block tampoco)
	// Chequeamos primero si existe.
	var exists bool
	err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}
	if !exists {
		_, err = conn.Exec(ctx, fmt.Sprintf(
			"CREATE DATABASE %s OWNER %s",
			safeDB, safeUser,
		))
		if err != nil {
			return fmt.Errorf("create database %s: %w", dbName, err)
		}
	}

	// Habilitar extensiones básicas en la nueva DB.
	// Necesitamos conectarnos a la nueva DB para esto.
	newDBDSN := r.replaceDBInDSN(adminDSN, dbName)
	newConn, err := pgx.Connect(ctx, newDBDSN)
	if err != nil {
		return fmt.Errorf("connect to new database %s: %w", dbName, err)
	}
	defer newConn.Close(ctx)

	_, _ = newConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto")
	_, _ = newConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis")

	return nil
}

// buildAdminDSN construye un DSN de superuser a partir del POSTGRES_USER/PASSWORD
// que están en el entorno (son las credenciales del contenedor db).
func (r *projectRepo) buildAdminDSN() (string, error) {
	// Parseamos el DATABASE_URL de la app para extraer host:port.
	cfg, err := pgxpool.ParseConfig(r.env.DATABASE_URL)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	host := cfg.ConnConfig.Host
	port := cfg.ConnConfig.Port

	// Usamos las credenciales del superuser (POSTGRES_USER/POSTGRES_PASSWORD)
	// que el contenedor db expone. Las leemos del entorno porque no están
	// en Conf (son config del container, no de la app Go).
	adminUser := envOrDefault("POSTGRES_USER", "postgres")
	adminPassword := envOrDefault("POSTGRES_PASSWORD", "")

	return fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable",
		adminUser, adminPassword, host, port), nil
}

func (r *projectRepo) replaceDBInDSN(dsn, newDB string) string {
	// Simple: replace /postgres? with /newdb?
	idx := strings.LastIndex(dsn, "/postgres")
	if idx >= 0 {
		return dsn[:idx] + "/" + newDB + dsn[idx+len("/postgres"):]
	}
	return dsn
}

// quoteLiteral escapa un string para usarlo como literal SQL (previene SQL injection).
func quoteLiteral(s string) string {
	// Reemplazar comillas simples por dobles
	escaped := strings.ReplaceAll(s, "'", "''")
	// Reemplazar backslashes
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	return "'" + escaped + "'"
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
