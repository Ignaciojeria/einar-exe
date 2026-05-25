package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"einar-exe/internal/domain"
	"einar-exe/internal/middleware"
	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

var _ = ioc.Register(apiProjectsHandler)

type ProjectCreateRequest struct {
	Name       string `json:"name"`
	Public     bool   `json:"public"`
	Visibility string `json:"visibility"`
}

type ProjectCreateResponse struct {
	ProjectID          string `json:"projectId"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Path               string `json:"path"`
	Subdomain          string `json:"subdomain"`
	Status             string `json:"status"`
	MutagenDestination string `json:"mutagenDestination,omitempty"`
	MutagenSessionName string `json:"mutagenSessionName,omitempty"`
	VMName             string `json:"vmName,omitempty"`
	VMHTTPSURL         string `json:"vmHttpsUrl,omitempty"`
	VMSshDest          string `json:"vmSshDest,omitempty"`
	VMSshPrivateKey    string `json:"vmSshPrivateKey,omitempty"`
	ProjectAPIToken    string `json:"projectApiToken,omitempty"`
	// Credenciales de la DB aislada del proyecto
	DBName     string `json:"dbName,omitempty"`
	DBUser     string `json:"dbUser,omitempty"`
	DBPassword string `json:"dbPassword,omitempty"`
	DBHost     string `json:"dbHost,omitempty"`
	DBPort     string `json:"dbPort,omitempty"`
}

func apiProjectsHandler(
	api *APIGroup,
	users domain.UserRepo,
	tenants domain.TenantRepo,
	projects domain.ProjectRepo,
	env environment.Conf,
) {
	fuego.Post(api.Server, "/projects", func(c fuego.ContextWithBody[ProjectCreateRequest]) (ProjectCreateResponse, error) {
		body, err := c.Body()
		if err != nil {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusBadRequest, Title: "invalid body", Detail: err.Error()}
		}

		name := strings.TrimSpace(body.Name)
		if name == "" {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusUnprocessableEntity, Title: "name is required"}
		}

		slug := slugify(name)
		if !isValidSlug(slug) {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusUnprocessableEntity, Title: "invalid name", Detail: "could not derive a valid slug"}
		}
		if isReservedSlug(slug) {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusUnprocessableEntity, Title: "invalid name", Detail: "slug is reserved"}
		}

		if !middleware.HasScope(c.Context(), "projects:create") {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusForbidden, Title: "missing scope", Detail: "projects:create scope is required"}
		}

		user, tenant, err := requireUserTenant(c.Context(), users, tenants)
		if err != nil {
			return ProjectCreateResponse{}, err
		}
		if err := requireOwnerOrAdmin(user); err != nil {
			return ProjectCreateResponse{}, err
		}

		baseDir := strings.TrimSpace(env.PROJECTS_BASE_DIR)
		if baseDir == "" {
			baseDir = "projects"
		}
		projectPath, err := safeProjectPath(baseDir, slug)
		if err != nil {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "invalid project path", Detail: err.Error()}
		}

		if _, statErr := os.Stat(projectPath); statErr == nil {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusConflict, Title: "project already exists"}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not verify project path", Detail: statErr.Error()}
		}

		domainName := baseDomain(env)
		if domainName == "" {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "base domain not configured"}
		}
		subdomain := fmt.Sprintf("%s.%s", slug, domainName)

		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not create project directory", Detail: err.Error()}
		}
		if err := scaffoldProject(projectPath, name, slug); err != nil {
			_ = os.RemoveAll(projectPath)
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not scaffold project", Detail: err.Error()}
		}

		isPublic := body.Public || strings.ToLower(strings.TrimSpace(body.Visibility)) == "public"
		vmInfo, err := maybeProvisionProjectVM(c.Context(), env, slug, subdomain, isPublic)
		if err != nil {
			_ = os.RemoveAll(projectPath)
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusBadGateway, Title: "could not provision project vm", Detail: err.Error()}
		}

		status := "ready"
		if vmInfo != nil && strings.TrimSpace(vmInfo.Status) != "" {
			status = strings.TrimSpace(vmInfo.Status)
		}

		// Generar credenciales de DB aislada para el proyecto.
		// user = slug_shortid (max 63 chars, Postgres limit)
		// password = UUID v4
		shortID := generateShortID()
		dbUser := sanitizeDBIdent(fmt.Sprintf("%s_%s", slug, shortID))
		dbPassword := uuid.New().String()
		dbName := dbUser // misma convención: db_name == db_user

		p, err := projects.Create(c.Context(), tenant.ID, name, slug, projectPath, subdomain, status, dbName, dbUser, dbPassword)
		if err != nil {
			_ = os.RemoveAll(projectPath)
			if errors.Is(err, domain.ErrConflict) {
				return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusConflict, Title: "project already exists"}
			}
			if errors.Is(err, domain.ErrInvalidArg) {
				return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusUnprocessableEntity, Title: "invalid project data"}
			}
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Title: "could not create project", Detail: err.Error()}
		}

		resp := ProjectCreateResponse{
			ProjectID:          p.ID.String(),
			Name:               p.Name,
			Slug:               p.Slug,
			Path:               p.Path,
			Subdomain:          p.Subdomain,
			Status:             p.Status,
			DBName:             p.DBName,
			DBUser:             p.DBUser,
			DBPassword:         p.DBPassword,
			DBHost:             "db",
			DBPort:             "5432",
			MutagenDestination: buildMutagenDestination(env, p.Path),
			MutagenSessionName: p.Slug,
		}
		if vmInfo != nil {
			resp.VMName = vmInfo.VMName
			resp.VMHTTPSURL = vmInfo.HTTPSURL
			resp.VMSshDest = normalizeMutagenDestination(vmInfo.SSHDest, env, p.Path)
			resp.VMSshPrivateKey = vmInfo.SSHPrivateKey
			resp.ProjectAPIToken = vmInfo.APIToken
			if strings.TrimSpace(resp.MutagenDestination) == "" {
				resp.MutagenDestination = resp.VMSshDest
			}
		}
		return resp, nil
	})
}

func scaffoldProject(projectPath, name, slug string) error {
	if err := os.MkdirAll(filepath.Join(projectPath, "src"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projectPath, "README.md"), []byte("# "+name+"\n\nProject slug: `"+slug+"`\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".env"), []byte("PROJECT_SLUG="+slug+"\n"), 0o600); err != nil {
		return err
	}
	return nil
}

func safeProjectPath(baseDir, slug string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	projectPath := filepath.Join(baseAbs, slug)
	rel, err := filepath.Rel(baseAbs, projectPath)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.New("path traversal detected")
	}
	return projectPath, nil
}

type provisionVMResponse struct {
	VMName       string `json:"vm_name"`
	HTTPSURL     string `json:"https_url"`
	SSHDest      string `json:"ssh_dest"`
	Status       string `json:"status"`
	SSHPrivateKey string `json:"ssh_private_key"`
	APIToken     string `json:"api_token"`
}

func maybeProvisionProjectVM(ctx context.Context, env environment.Conf, slug, subdomain string, public bool) (*provisionVMResponse, error) {
	if strings.TrimSpace(env.EXE_API_TOKEN) != "" {
		return provisionProjectVMByHTTP(ctx, env, slug, subdomain, public)
	}
	target := strings.TrimSpace(env.VM_PROVISION_SSH_TARGET)
	if target == "" {
		return nil, nil
	}
	createCmd := strings.TrimSpace(env.VM_PROVISION_CREATE_CMD)
	if createCmd == "" {
		createCmd = "new"
	}
	timeoutSec := env.VM_PROVISION_TIMEOUT_SEC
	if timeoutSec <= 0 {
		timeoutSec = 90
	}
	pctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(pctx, "ssh", target, createCmd,
		"--name", slug,
		"--domain", subdomain,
		"--json",
	)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("provisioner command failed: %w | output: %s", err, strings.TrimSpace(string(raw)))
	}
	return parseProvisionVMResponse(raw)
}

func provisionProjectVMByHTTP(ctx context.Context, env environment.Conf, slug, subdomain string, public bool) (*provisionVMResponse, error) {
	timeoutSec := env.VM_PROVISION_TIMEOUT_SEC
	if timeoutSec <= 0 {
		timeoutSec = 90
	}
	pctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	endpoint := strings.TrimSpace(env.EXE_API_URL)
	if endpoint == "" {
		endpoint = "https://exe.dev/exec"
	}
	command := fmt.Sprintf("new --name=%s --json", slug)
	req, err := http.NewRequestWithContext(pctx, http.MethodPost, endpoint, bytes.NewBufferString(command))
	if err != nil {
		return nil, err
	}
	apiToken := strings.TrimSpace(env.EXE_API_TOKEN)
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("provisioner http failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	result, err := parseProvisionVMResponse(raw)
	if err != nil {
		return nil, err
	}

	if public && result.VMName != "" {
		_ = exeAPIPost(pctx, endpoint, apiToken, fmt.Sprintf("share set-public %s", result.VMName))
		_ = exeAPIPost(pctx, endpoint, apiToken, fmt.Sprintf("share port %s 8000", result.VMName))
	}

	return result, nil
}

func exeAPIPost(ctx context.Context, endpoint, token, command string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(command))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("exe api %q: status %d", command, resp.StatusCode)
	}
	return nil
}

func parseProvisionVMResponse(raw []byte) (*provisionVMResponse, error) {
	var out provisionVMResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse provisioner response: %w", err)
	}
	if strings.TrimSpace(out.Status) == "" {
		out.Status = "ready"
	}
	status := strings.ToLower(strings.TrimSpace(out.Status))
	if status == "error" || status == "failed" {
		return nil, fmt.Errorf("provisioner returned status=%q", out.Status)
	}
	if strings.TrimSpace(out.VMName) == "" {
		return nil, fmt.Errorf("provisioner response missing vm_name")
	}
	return &out, nil
}

func normalizeMutagenDestination(raw string, env environment.Conf, projectPath string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		// Si ya viene como ssh:// o docker:// lo respetamos.
		return value
	}

	user := strings.TrimSpace(env.PROJECTS_SYNC_SSH_USER)
	if user == "" {
		user = "root"
	}
	port := strings.TrimSpace(env.PROJECTS_SYNC_SSH_PORT)
	if port == "" || port == "22" {
		return fmt.Sprintf("ssh://%s@%s%s", user, value, projectPath)
	}
	return fmt.Sprintf("ssh://%s@%s:%s%s", user, value, port, projectPath)
}

func buildMutagenDestination(env environment.Conf, projectPath string) string {
	host := strings.TrimSpace(env.PROJECTS_SYNC_SSH_HOST)
	user := strings.TrimSpace(env.PROJECTS_SYNC_SSH_USER)
	if host == "" || user == "" {
		return ""
	}
	port := strings.TrimSpace(env.PROJECTS_SYNC_SSH_PORT)
	if port == "" || port == "22" {
		return fmt.Sprintf("ssh://%s@%s%s", user, host, projectPath)
	}
	return fmt.Sprintf("ssh://%s@%s:%s%s", user, host, port, projectPath)
}

func baseDomain(env environment.Conf) string {
	if strings.TrimSpace(env.PROJECTS_BASE_DOMAIN) != "" {
		return strings.TrimSpace(env.PROJECTS_BASE_DOMAIN)
	}
	u, err := url.Parse(env.APP_PUBLIC_URL)
	if err != nil {
		return ""
	}
	host := u.Host
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(name)
	s = slugCleaner.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	return s
}

func isValidSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 32 {
		return false
	}
	return regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`).MatchString(slug)
}

func isReservedSlug(slug string) bool {
	reserved := map[string]struct{}{
		"www": {}, "api": {}, "admin": {}, "app": {}, "root": {},
	}
	_, exists := reserved[slug]
	return exists
}

// generateShortID genera un ID corto hex de 4 bytes (8 chars).
func generateShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sanitizeDBIdent limpia un nombre para usarse como identifier de Postgres.
// Solo permite [a-z0-9_], trunca a 63 chars (límite de Postgres).
func sanitizeDBIdent(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		}
	}
	result := b.String()
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}
