package environment

import (
	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewConf)

// Conf agrupa la configuración del proceso `app` de einar-exe.
//
// Reglas (twelve-factor III — Config):
//   - Solo variables que consume el binario Go.
//   - Las credenciales del contenedor `db` (POSTGRES_*) o del servidor
//     `casdoor` (CASDOOR_DRIVER, CASDOOR_ADMIN_*, etc.) NO viven aquí:
//     son configuración interna de esos servicios y la app no debe verlas.
//   - Las variables marcadas como `required:"true"` hacen que la app falle
//     rápido al arranque si no están definidas.
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
	// Si se define host+user, /api/projects responderá `mutagenDestination`.
	PROJECTS_SYNC_SSH_HOST string `env:"PROJECTS_SYNC_SSH_HOST"`
	PROJECTS_SYNC_SSH_USER string `env:"PROJECTS_SYNC_SSH_USER" envDefault:"exedev"`
	PROJECTS_SYNC_SSH_PORT string `env:"PROJECTS_SYNC_SSH_PORT" envDefault:"22"`
	// Template para el path remoto del proyecto en la VM.
	// {slug} se reemplaza por el slug del proyecto.
	PROJECTS_REMOTE_PATH_TEMPLATE string `env:"PROJECTS_REMOTE_PATH_TEMPLATE" envDefault:"/home/exedev/workspace/{slug}"`

	// Provisioner opcional para crear VM aislada por proyecto (MVP).
	// Si VM_PROVISION_SSH_TARGET está vacío, /api/projects mantiene modo local.
	VM_PROVISION_SSH_TARGET string `env:"VM_PROVISION_SSH_TARGET"`
	VM_PROVISION_CREATE_CMD string `env:"VM_PROVISION_CREATE_CMD" envDefault:"new"`
	VM_PROVISION_TIMEOUT_SEC int    `env:"VM_PROVISION_TIMEOUT_SEC" envDefault:"90"`

	// Provisioner HTTP API opcional (preferido sobre SSH si hay token).
	EXE_API_URL   string `env:"EXE_API_URL" envDefault:"https://exe.dev/exec"`
	EXE_API_TOKEN string `env:"EXE_API_TOKEN"`

	// ------------------------------------------------------------
	// Backing service: Postgres
	// ------------------------------------------------------------
	// Cadena de conexión completa (twelve-factor IV — recurso enchufable).
	// Cambiar de Postgres local a uno gestionado debe ser solo cambiar esta URL.
	DATABASE_URL string `env:"DATABASE_URL,required"`

	// Postgres superuser credentials for admin operations (CREATE ROLE, CREATE DATABASE).
	// These should match the POSTGRES_USER and POSTGRES_PASSWORD from docker-compose.yml.
	POSTGRES_USER     string `env:"POSTGRES_USER,required"`
	POSTGRES_PASSWORD string `env:"POSTGRES_PASSWORD,required"`

	// ------------------------------------------------------------
	// Backing service: Casdoor (IAM)
	// ------------------------------------------------------------
	// Endpoint interno (server-to-server) usado para intercambio de tokens,
	// llamadas a la API y descarga de JWKS. Resuelve dentro de la red Docker.
	CASDOOR_ENDPOINT_INTERNAL string `env:"CASDOOR_ENDPOINT_INTERNAL,required"`

	// Origin público de Casdoor: la URL a la que se redirige el navegador
	// del usuario en el flujo OAuth (login screen).
	CASDOOR_ORIGIN string `env:"CASDOOR_ORIGIN" envDefault:"http://localhost:8000"`

	// Identificadores lógicos en Casdoor.
	CASDOOR_ORG_NAME string `env:"CASDOOR_ORG_NAME" envDefault:"einar"`
	CASDOOR_APP_NAME string `env:"CASDOOR_APP_NAME" envDefault:"einar-app"`

	// ------------------------------------------------------------
	// Backing service: OpenObserve (logs/metrics/traces)
	// ------------------------------------------------------------
	// Endpoint interno (server-to-server). Incluye el `ZO_BASE_URI`
	// que se setea en el container, ya que toda la API de OO vive bajo
	// ese prefijo (ej. http://openobserve:5080/o2).
	OPENOBSERVE_ENDPOINT_INTERNAL string `env:"OPENOBSERVE_ENDPOINT_INTERNAL,required"`

	// Credenciales del root user de OpenObserve. Son las mismas que el
	// container recibe (ZO_ROOT_USER_*); las re-leemos en el binario Go
	// porque las usamos como admin auth para aprovisionar orgs.
	ZO_ROOT_USER_EMAIL    string `env:"ZO_ROOT_USER_EMAIL,required"`
	ZO_ROOT_USER_PASSWORD string `env:"ZO_ROOT_USER_PASSWORD,required"`

	// ------------------------------------------------------------
	// Backing service: Metabase (BI / dashboards)
	// ------------------------------------------------------------
	METABASE_ENDPOINT_INTERNAL string `env:"METABASE_ENDPOINT_INTERNAL,required"`
	// Admin user que setup.sh crea via POST /api/setup. El binario Go
	// hace login con esto para obtener un session token y aprovisionar
	// groups/users en signup.
	METABASE_ADMIN_EMAIL    string `env:"METABASE_ADMIN_EMAIL,required"`
	METABASE_ADMIN_PASSWORD string `env:"METABASE_ADMIN_PASSWORD,required"`

	// ------------------------------------------------------------
	// OAuth2
	// ------------------------------------------------------------
	// Estrategia MVP: JWT puro de Casdoor en cookie HttpOnly. La app no
	// firma sesiones propias, por eso NO existe APP_SESSION_SECRET aquí.
	// Ver docs/casdoor-integration-plan.md §12 (decisión registrada).
	CASDOOR_CLIENT_ID      string `env:"CASDOOR_CLIENT_ID,required"`
	CASDOOR_CLIENT_SECRET  string `env:"CASDOOR_CLIENT_SECRET,required"`
	APP_OAUTH_REDIRECT_URI string `env:"APP_OAUTH_REDIRECT_URI,required"`

	// ------------------------------------------------------------
	// einar JWT signer (para tokens de embedded apps)
	// ------------------------------------------------------------
	// Clave privada RSA en formato PEM (PKCS1 o PKCS8). Puede ser el PEM
	// directo o un path a un archivo. Si está vacía, se genera una clave
	// ephemeral al startup (OK para dev, NO para prod — los tokens emitidos
	// no sobreviven al restart).
	EINAR_JWT_PRIVATE_KEY string `env:"EINAR_JWT_PRIVATE_KEY"`

	// ------------------------------------------------------------
	// Signup allowlist (anti-abuse)
	// ------------------------------------------------------------
	// Lista CSV de emails permitidos para crear tenant nuevo.
	// Si está vacía, signup está abierto (NO recomendado en prod).
	// Ej: "juan@x.com,maria@y.com"
	EINAR_SIGNUP_ALLOWLIST string `env:"EINAR_SIGNUP_ALLOWLIST"`

	// Máximo de VMs por tenant. 0 o negativo = sin límite.
	// En modo abierto (allowlist vacía) esto es el único freno contra abuse.
	EINAR_MAX_VMS_PER_TENANT int `env:"EINAR_MAX_VMS_PER_TENANT" envDefault:"2"`
}

func NewConf() (Conf, error) {
	return Parse[Conf]()
}
