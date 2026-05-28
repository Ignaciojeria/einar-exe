package environment

import (
	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewConf)

// Conf agrupa la configuración del proceso `app` de einar-exe.
//
// Reglas (twelve-factor III — Config):
//   - Solo variables que consume el binario Go.
//   - Las variables marcadas como `required:"true"` hacen que la app falle
//     rápido al arranque si no están definidas.
//
// Authentication is handled by exe.dev proxy headers (X-ExeDev-UserID,
// X-ExeDev-Email). No Casdoor/OIDC configuration needed.
type Conf struct {
	// ------------------------------------------------------------
	// App
	// ------------------------------------------------------------
	APP_ENV              string `env:"APP_ENV"        envDefault:"development"` // development | staging | production
	APP_PORT             string `env:"APP_PORT"       envDefault:"8080"`
	APP_PUBLIC_URL       string `env:"APP_PUBLIC_URL" envDefault:"http://localhost:8080"`
	LOG_LEVEL            string `env:"LOG_LEVEL"      envDefault:"info"` // debug | info | warn | error
	PROJECT_NAME         string `env:"PROJECT_NAME"   envDefault:"einar-exe"`
	VERSION              string `env:"VERSION"`
	PROJECTS_BASE_DIR    string `env:"PROJECTS_BASE_DIR"    envDefault:"projects"`
	PROJECTS_BASE_DOMAIN string `env:"PROJECTS_BASE_DOMAIN"`
	// Config opcional para sugerir endpoint de sync (Mutagen SSH) al CLI.
	PROJECTS_SYNC_SSH_HOST string `env:"PROJECTS_SYNC_SSH_HOST"`
	PROJECTS_SYNC_SSH_USER string `env:"PROJECTS_SYNC_SSH_USER" envDefault:"exedev"`
	PROJECTS_SYNC_SSH_PORT string `env:"PROJECTS_SYNC_SSH_PORT" envDefault:"22"`
	PROJECTS_REMOTE_PATH_TEMPLATE string `env:"PROJECTS_REMOTE_PATH_TEMPLATE" envDefault:"/home/exedev/workspace/{slug}"`

	// Provisioner opcional para crear VM aislada por proyecto (MVP).
	VM_PROVISION_SSH_TARGET  string `env:"VM_PROVISION_SSH_TARGET"`
	VM_PROVISION_CREATE_CMD  string `env:"VM_PROVISION_CREATE_CMD" envDefault:"new"`
	VM_PROVISION_TIMEOUT_SEC int    `env:"VM_PROVISION_TIMEOUT_SEC" envDefault:"90"`

	// Provisioner HTTP API opcional (preferido sobre SSH si hay token).
	EXE_API_URL   string `env:"EXE_API_URL" envDefault:"https://exe.dev/exec"`
	EXE_API_TOKEN string `env:"EXE_API_TOKEN"`

	// ------------------------------------------------------------
	// Backing service: Postgres
	// ------------------------------------------------------------
	DATABASE_URL      string `env:"DATABASE_URL,required"`
	POSTGRES_USER     string `env:"POSTGRES_USER,required"`
	POSTGRES_PASSWORD string `env:"POSTGRES_PASSWORD,required"`

	// ------------------------------------------------------------
	// Backing service: OpenObserve (logs/metrics/traces)
	// ------------------------------------------------------------
	OPENOBSERVE_ENDPOINT_INTERNAL string `env:"OPENOBSERVE_ENDPOINT_INTERNAL,required"`
	ZO_ROOT_USER_EMAIL            string `env:"ZO_ROOT_USER_EMAIL,required"`
	ZO_ROOT_USER_PASSWORD         string `env:"ZO_ROOT_USER_PASSWORD,required"`

	// ------------------------------------------------------------
	// Backing service: Metabase (BI / dashboards)
	// ------------------------------------------------------------
	METABASE_ENDPOINT_INTERNAL string `env:"METABASE_ENDPOINT_INTERNAL,required"`
	METABASE_ADMIN_EMAIL       string `env:"METABASE_ADMIN_EMAIL,required"`
	METABASE_ADMIN_PASSWORD    string `env:"METABASE_ADMIN_PASSWORD,required"`

	// ------------------------------------------------------------
	// einar JWT signer (para tokens de embedded apps)
	// ------------------------------------------------------------
	EINAR_JWT_PRIVATE_KEY string `env:"EINAR_JWT_PRIVATE_KEY"`
}

func NewConf() (Conf, error) {
	return Parse[Conf]()
}
