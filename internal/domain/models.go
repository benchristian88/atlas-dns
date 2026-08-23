package domain

import "time"

type UserRole string

const RoleAdministrator UserRole = "administrator"

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         UserRole
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	CSRFHash   []byte
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
	IPMetadata string
	UserAgent  string
}

type Cluster struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Description          string               `json:"description"`
	ReconciliationPolicy ReconciliationPolicy `json:"reconciliationPolicy"`
	ActiveRevisionID     *string              `json:"activeRevisionId,omitempty"`
	Version              int                  `json:"version"`
	CreatedAt            time.Time            `json:"createdAt"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

type ReconciliationPolicy string

const (
	ReconciliationEnforce ReconciliationPolicy = "enforce"
	ReconciliationAlert   ReconciliationPolicy = "alert"
	ReconciliationManual  ReconciliationPolicy = "manual"
)

func ValidReconciliationPolicy(policy ReconciliationPolicy) bool {
	return policy == ReconciliationEnforce || policy == ReconciliationAlert || policy == ReconciliationManual
}

type NodeHealth string

const (
	NodeUnknown      NodeHealth = "unknown"
	NodeHealthy      NodeHealth = "healthy"
	NodeUnreachable  NodeHealth = "unreachable"
	NodeIncompatible NodeHealth = "incompatible"
	NodeDisabled     NodeHealth = "disabled"
)

type CertificatePolicy string

const (
	CertificateSystemTrust  CertificatePolicy = "system"
	CertificateCustomCA     CertificatePolicy = "custom_ca"
	CertificateInsecureHTTP CertificatePolicy = "insecure_http"
)

type Compatibility string

const (
	CompatibilityUnknown     Compatibility = "unknown"
	CompatibilitySupported   Compatibility = "supported"
	CompatibilityUnsupported Compatibility = "unsupported"
)

type Node struct {
	ID                  string            `json:"id"`
	ClusterID           string            `json:"clusterId"`
	Name                string            `json:"name"`
	BaseURL             string            `json:"baseUrl"`
	CertificatePolicy   CertificatePolicy `json:"certificatePolicy"`
	Enabled             bool              `json:"enabled"`
	MaintenanceMode     bool              `json:"maintenanceMode"`
	HealthStatus        NodeHealth        `json:"healthStatus"`
	CompatibilityStatus Compatibility     `json:"compatibilityStatus"`
	Version             string            `json:"version,omitempty"`
	LastSeenAt          *time.Time        `json:"lastSeenAt,omitempty"`
	LastPolledAt        *time.Time        `json:"lastPolledAt,omitempty"`
	LatencyMS           *int              `json:"latencyMs,omitempty"`
	LastErrorCode       string            `json:"lastErrorCode,omitempty"`
	AppliedRevisionID   *string           `json:"appliedRevisionId,omitempty"`
	AppliedHash         string            `json:"appliedHash,omitempty"`
	ConvergenceStatus   string            `json:"convergenceStatus"`
	LastReconciledAt    *time.Time        `json:"lastReconciledAt,omitempty"`
	RecordVersion       int               `json:"recordVersion"`
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

type NodeCredentials struct {
	Username string
	Password string
}

type EncryptedCredentials struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
	Algorithm  string
}

// EncryptedPayload stores short-lived sensitive operational-command input.
// The associated-data scope binds the ciphertext to its command resource.
type EncryptedPayload struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
	Algorithm  string
}

type NodeSecretMaterial struct {
	Credentials EncryptedCredentials
	CustomCAPEM string
}

type NodeRecord struct {
	Node    Node
	Secrets NodeSecretMaterial
}

type NodeProbeRequest struct {
	BaseURL           string
	CertificatePolicy CertificatePolicy
	CustomCAPEM       string
	Credentials       NodeCredentials
}

type NodeProbeResult struct {
	Version                      string        `json:"version"`
	Compatibility                Compatibility `json:"compatibility"`
	Running                      bool          `json:"running"`
	ProtectionEnabled            bool          `json:"protectionEnabled"`
	ProtectionDisabledDurationMS int64         `json:"protectionDisabledDurationMillis"`
	LatencyMS                    int           `json:"latencyMs"`
}

type AuditEvent struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actorType"`
	ActorUserID  *string        `json:"actorUserId,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   *string        `json:"resourceId,omitempty"`
	RequestID    string         `json:"requestId"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}
