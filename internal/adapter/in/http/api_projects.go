package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

var _ = ioc.Register(apiProjectsHandler)

type ProjectCreateRequest struct {
	Name string `json:"name"`
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

		vmInfo, err := maybeProvisionProjectVM(c.Context(), env, slug, subdomain)
		if err != nil {
			_ = os.RemoveAll(projectPath)
			return ProjectCreateResponse{}, fuego.HTTPError{Status: http.StatusBadGateway, Title: "could not provision project vm", Detail: err.Error()}
		}

		status := "ready"
		if vmInfo != nil && strings.TrimSpace(vmInfo.Status) != "" {
			status = strings.TrimSpace(vmInfo.Status)
		}

		p, err := projects.Create(c.Context(), tenant.ID, name, slug, projectPath, subdomain, status)
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
			MutagenDestination: buildMutagenDestination(env, p.Path),
			MutagenSessionName: p.Slug,
		}
		if vmInfo != nil {
			resp.VMName = vmInfo.VMName
			resp.VMHTTPSURL = vmInfo.HTTPSURL
			resp.VMSshDest = vmInfo.SSHDest
			resp.VMSshPrivateKey = vmInfo.SSHPrivateKey
			resp.ProjectAPIToken = vmInfo.APIToken
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

func maybeProvisionProjectVM(ctx context.Context, env environment.Conf, slug, subdomain string) (*provisionVMResponse, error) {
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
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out provisionVMResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse provisioner response: %w", err)
	}
	if strings.TrimSpace(out.Status) == "" {
		out.Status = "ready"
	}
	return &out, nil
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
