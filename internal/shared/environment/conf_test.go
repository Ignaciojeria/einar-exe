package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// requiredEnvs son las variables sin envDefault que la app exige al arrancar.
// Si una falta, env.Parse devuelve error (twelve-factor III: fail fast).
var requiredEnvs = map[string]string{
	"DATABASE_URL": "postgres://einar:test@localhost:5432/einar?sslmode=disable",
}

// chdirToNoEnv evita que parse.go cargue el .env real del repo durante el test.
func chdirToNoEnv(t *testing.T) {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	noenv := filepath.Join(filepath.Dir(f), "testdata", "noenv")
	if err := os.MkdirAll(noenv, 0o755); err != nil {
		t.Fatalf("mkdir testdata/noenv: %v", err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(noenv); err != nil {
		t.Fatalf("chdir noenv: %v", err)
	}
}

func setRequiredEnvs(t *testing.T) {
	t.Helper()
	for k, v := range requiredEnvs {
		t.Setenv(k, v)
	}
}

func TestNewConf_DefaultValues(t *testing.T) {
	chdirToNoEnv(t)
	setRequiredEnvs(t)

	// Aseguramos que los con default no estén seteados desde fuera.
	for _, k := range []string{"APP_ENV", "APP_PORT", "LOG_LEVEL", "PROJECT_NAME"} {
		os.Unsetenv(k)
	}

	conf, err := NewConf()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.APP_PORT != "8080" {
		t.Errorf("expected default APP_PORT=8080, got %q", conf.APP_PORT)
	}
	if conf.APP_ENV != "development" {
		t.Errorf("expected default APP_ENV=development, got %q", conf.APP_ENV)
	}
	if conf.LOG_LEVEL != "info" {
		t.Errorf("expected default LOG_LEVEL=info, got %q", conf.LOG_LEVEL)
	}
	if conf.PROJECT_NAME != "einar-exe" {
		t.Errorf("expected default PROJECT_NAME=einar-exe, got %q", conf.PROJECT_NAME)
	}
}

func TestNewConf_CustomEnvs(t *testing.T) {
	chdirToNoEnv(t)
	setRequiredEnvs(t)

	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PROJECT_NAME", "einar-exe")
	t.Setenv("VERSION", "2.0.0")

	conf, err := NewConf()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.APP_PORT != "9090" {
		t.Errorf("expected APP_PORT=9090, got %q", conf.APP_PORT)
	}
	if conf.APP_ENV != "production" {
		t.Errorf("expected APP_ENV=production, got %q", conf.APP_ENV)
	}
	if conf.VERSION != "2.0.0" {
		t.Errorf("expected VERSION=2.0.0, got %q", conf.VERSION)
	}
}

func TestNewConf_FailsWhenRequiredMissing(t *testing.T) {
	chdirToNoEnv(t)
	// Aseguramos que ninguna required esté definida.
	for k := range requiredEnvs {
		os.Unsetenv(k)
	}

	if _, err := NewConf(); err == nil {
		t.Fatal("expected error when required envs are missing, got nil")
	}
}
