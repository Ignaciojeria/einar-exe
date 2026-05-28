package domain

import "time"

// ProjectRuntimeConfig es el JSON canónico que describe toda la
// infraestructura aprovisionada para un proyecto. Se persiste como
// runtime.json en el directorio del proyecto y se devuelve en la
// response de POST /api/projects.
//
// Diseño:
//   - Secretos NUNCA van inline: se referencian vía *SecretRef
//     (path lógico que un secret manager resuelve).
//   - version: permite migrar el schema del JSON a futuro.
type ProjectRuntimeConfig struct {
	Version   int                `json:"version"`
	ProjectID string             `json:"projectId"`
	Slug      string             `json:"slug"`
	Workspace RuntimeWorkspace   `json:"workspace"`
	VM        *RuntimeVM         `json:"vm,omitempty"`
	Sync      *RuntimeSync       `json:"sync,omitempty"`
	Database  *RuntimeDatabase   `json:"database,omitempty"`
	Secrets   *RuntimeSecrets    `json:"secrets,omitempty"`
	Metadata  RuntimeMetadata    `json:"metadata"`
}

type RuntimeWorkspace struct {
	Branch string `json:"branch"`
	Mode   string `json:"mode"`
}

type RuntimeVM struct {
	Name              string `json:"name"`
	HTTPSURL          string `json:"httpsUrl"`
	SSHDestination    string `json:"sshDestination"`
	RemoteProjectPath string `json:"remoteProjectPath"`
}

type RuntimeSync struct {
	Provider    string `json:"provider"`
	Destination string `json:"destination"`
	SessionName string `json:"sessionName"`
	IgnoreVCS   bool   `json:"ignoreVCS"`
}

type RuntimeDatabase struct {
	Name              string `json:"name"`
	User              string `json:"user"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	PasswordSecretRef string `json:"passwordSecretRef"`
}

type RuntimeSecrets struct {
	ProjectAPITokenSecretRef string `json:"projectApiTokenSecretRef,omitempty"`
	SSHPrivateKeySecretRef   string `json:"sshPrivateKeySecretRef,omitempty"`
	DBPasswordSecretRef      string `json:"dbPasswordSecretRef,omitempty"`

	// Inline secret material. SOLO se devuelve en la respuesta de
	// creación del proyecto (one-shot). El CLI los guarda localmente.
	// En reads posteriores estos campos quedan vacíos y solo se devuelven
	// los *SecretRef. Si el user los pierde, hay que rotar.
	ProjectAPIToken string `json:"projectApiToken,omitempty"`
	SSHPrivateKey   string `json:"sshPrivateKey,omitempty"`
	DBPassword      string `json:"dbPassword,omitempty"`
}

type RuntimeMetadata struct {
	OwnerUserID string    `json:"ownerUserId"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
