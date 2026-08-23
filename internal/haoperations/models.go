package haoperations

import (
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type InstallationType string

const (
	InstallationNativeSystemd InstallationType = "native_systemd"
	InstallationDocker        InstallationType = "docker"
	InstallationHomeAssistant InstallationType = "home_assistant_addon"
	InstallationCustom        InstallationType = "custom"
	InstallationUnknown       InstallationType = "unknown"
)

func (value InstallationType) Valid() bool {
	switch value {
	case InstallationNativeSystemd, InstallationDocker, InstallationHomeAssistant, InstallationCustom, InstallationUnknown:
		return true
	default:
		return false
	}
}

type UpgradeSupport string

const (
	UpgradeGuided      UpgradeSupport = "guided"
	UpgradeUnsupported UpgradeSupport = "unsupported"
)

func SupportForInstallation(value InstallationType) UpgradeSupport {
	if value == InstallationNativeSystemd || value == InstallationDocker {
		return UpgradeGuided
	}
	return UpgradeUnsupported
}

type NodeSettings struct {
	NodeID           string           `json:"nodeId"`
	DNSProbeHost     string           `json:"dnsProbeHost"`
	DNSProbePort     int              `json:"dnsProbePort"`
	DNSProbeName     string           `json:"dnsProbeName"`
	DNSProbeType     string           `json:"dnsProbeType"`
	ExpectedRCode    int              `json:"expectedRcode"`
	ProbeUDP         bool             `json:"probeUdp"`
	ProbeTCP         bool             `json:"probeTcp"`
	InstallationType InstallationType `json:"installationType"`
	RecordVersion    int              `json:"recordVersion"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}

type DNSProbeResult struct {
	ID            string    `json:"id"`
	ClusterID     string    `json:"clusterId"`
	NodeID        string    `json:"nodeId"`
	Status        string    `json:"status"`
	UDPStatus     string    `json:"udpStatus"`
	TCPStatus     string    `json:"tcpStatus"`
	ResponseCode  *int      `json:"responseCode,omitempty"`
	LatencyMS     *int      `json:"latencyMs,omitempty"`
	AddressFamily string    `json:"addressFamily,omitempty"`
	ErrorCode     string    `json:"errorCode,omitempty"`
	ProbedAt      time.Time `json:"probedAt"`
}

type Event struct {
	ID         string         `json:"id"`
	ClusterID  string         `json:"clusterId"`
	NodeID     *string        `json:"nodeId,omitempty"`
	EventType  string         `json:"eventType"`
	Severity   string         `json:"severity"`
	Summary    string         `json:"summary"`
	Details    map[string]any `json:"details"`
	OccurredAt time.Time      `json:"occurredAt"`
}

type HistoryItem struct {
	ID           string                      `json:"id"`
	Kind         string                      `json:"kind"`
	ClusterID    string                      `json:"clusterId"`
	NodeID       *string                     `json:"nodeId,omitempty"`
	EventType    string                      `json:"eventType"`
	Severity     string                      `json:"severity"`
	Summary      string                      `json:"summary"`
	Details      map[string]any              `json:"details"`
	OccurredAt   time.Time                   `json:"occurredAt"`
	Notification *NotificationHistoryOutcome `json:"notification,omitempty"`
}

type NotificationHistoryOutcome struct {
	ChannelID    *string    `json:"channelId,omitempty"`
	ChannelName  string     `json:"channelName"`
	Status       string     `json:"status"`
	AttemptCount int        `json:"attemptCount"`
	ErrorCode    string     `json:"errorCode,omitempty"`
	ErrorSummary string     `json:"errorSummary,omitempty"`
	HTTPStatus   *int       `json:"httpStatus,omitempty"`
	Test         bool       `json:"test"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

type HistoryQuery struct {
	ClusterID string
	NodeID    string
	Limit     int
	BeforeAt  *time.Time
	BeforeID  string
}

type HistoryRequest struct {
	ClusterID string
	NodeID    string
	Cursor    string
	Limit     int
}

type HistoryPage struct {
	Items      []HistoryItem `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
	HasMore    bool          `json:"hasMore"`
}

type CertificateState string

const (
	CertificateHealthy  CertificateState = "healthy"
	CertificateWarning  CertificateState = "warning"
	CertificateCritical CertificateState = "critical"
	CertificateExpired  CertificateState = "expired"
	CertificateUnknown  CertificateState = "unknown"
)

type Certificate struct {
	NodeID        string           `json:"nodeId"`
	NodeName      string           `json:"nodeName"`
	Subject       string           `json:"subject,omitempty"`
	Issuer        string           `json:"issuer,omitempty"`
	NotAfter      *time.Time       `json:"notAfter,omitempty"`
	DaysRemaining *int             `json:"daysRemaining,omitempty"`
	State         CertificateState `json:"state"`
	ObservedAt    *time.Time       `json:"observedAt,omitempty"`
}

type VersionState struct {
	NodeID            string           `json:"nodeId"`
	NodeName          string           `json:"nodeName"`
	InstalledVersion  string           `json:"installedVersion"`
	LatestVersion     string           `json:"latestVersion,omitempty"`
	Compatibility     string           `json:"compatibility"`
	InstallationType  InstallationType `json:"installationType"`
	UpgradeSupport    UpgradeSupport   `json:"upgradeSupport"`
	UpdateAvailable   bool             `json:"updateAvailable"`
	ReleaseCheckStale bool             `json:"releaseCheckStale"`
}

type HASummary struct {
	State                string         `json:"state"`
	TotalNodes           int            `json:"totalNodes"`
	ServingDNSNodes      int            `json:"servingDnsNodes"`
	APIReachableNodes    int            `json:"apiReachableNodes"`
	ConvergedNodes       int            `json:"convergedNodes"`
	MaintenanceNodes     int            `json:"maintenanceNodes"`
	CertificateWarnings  int            `json:"certificateWarnings"`
	UpdateAvailableNodes int            `json:"updateAvailableNodes"`
	Message              string         `json:"message"`
	Nodes                []HANodeStatus `json:"nodes"`
}

type HANodeStatus struct {
	NodeID      string     `json:"nodeId"`
	DNSStatus   string     `json:"dnsStatus"`
	UDPStatus   string     `json:"udpStatus"`
	TCPStatus   string     `json:"tcpStatus"`
	DNSProbedAt *time.Time `json:"dnsProbedAt,omitempty"`
	ErrorCode   string     `json:"errorCode,omitempty"`
}

type NodeLifecycle struct {
	GeneratedAt time.Time       `json:"generatedAt"`
	Settings    NodeSettings    `json:"settings"`
	DNS         *DNSProbeResult `json:"dns,omitempty"`
	Certificate Certificate     `json:"certificate"`
	Version     VersionState    `json:"version"`
	Events      []Event         `json:"events"`
}

type Check struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Required  bool   `json:"required"`
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message"`
}

type MaintenancePreflight struct {
	NodeID                   string  `json:"nodeId"`
	Allowed                  bool    `json:"allowed"`
	BreakGlassRequired       bool    `json:"breakGlassRequired"`
	HealthyDNSNodesRemaining int     `json:"healthyDnsNodesRemaining"`
	ExpectedRedundancy       string  `json:"expectedRedundancy"`
	ActiveDeployment         bool    `json:"activeDeployment"`
	OpenDrift                bool    `json:"openDrift"`
	ActiveDHCP               bool    `json:"activeDhcp"`
	Checks                   []Check `json:"checks"`
}

type ReturnValidation struct {
	NodeID    string  `json:"nodeId"`
	Succeeded bool    `json:"succeeded"`
	Checks    []Check `json:"checks"`
}

type Upgrade struct {
	ID               string           `json:"id"`
	ClusterID        string           `json:"clusterId"`
	NodeID           string           `json:"nodeId"`
	FromVersion      string           `json:"fromVersion"`
	TargetVersion    string           `json:"targetVersion"`
	InstallationType InstallationType `json:"installationType"`
	Mode             string           `json:"mode"`
	Status           string           `json:"status"`
	RequestedBy      string           `json:"requestedBy"`
	RequestID        string           `json:"requestId"`
	Preflight        map[string]any   `json:"preflight"`
	Validation       map[string]any   `json:"validation"`
	ErrorCode        string           `json:"errorCode,omitempty"`
	ErrorSummary     string           `json:"errorSummary,omitempty"`
	StartedAt        time.Time        `json:"startedAt"`
	CompletedAt      *time.Time       `json:"completedAt,omitempty"`
}

type ReleaseCache struct {
	Version       string    `json:"version"`
	ReleaseURL    string    `json:"releaseUrl"`
	Compatibility string    `json:"compatibility"`
	CheckedAt     time.Time `json:"checkedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	ErrorCode     string    `json:"errorCode,omitempty"`
}

type NotificationChannel struct {
	ID                 string    `json:"id"`
	ClusterID          string    `json:"clusterId"`
	Name               string    `json:"name"`
	ChannelType        string    `json:"channelType"`
	Enabled            bool      `json:"enabled"`
	DestinationSet     bool      `json:"destinationSet"`
	DestinationSummary string    `json:"destinationSummary"`
	SubscribedEvents   []string  `json:"subscribedEvents"`
	RecordVersion      int       `json:"recordVersion"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type NotificationChannelRecord struct {
	Channel     NotificationChannel
	Destination domain.EncryptedPayload
}

type NotificationDelivery struct {
	ID            string     `json:"id"`
	ChannelID     string     `json:"channelId"`
	EventID       string     `json:"eventId"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attemptCount"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	ErrorSummary  string     `json:"errorSummary,omitempty"`
	HTTPStatus    *int       `json:"httpStatus,omitempty"`
	NextAttemptAt *time.Time `json:"nextAttemptAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	Event         Event      `json:"event"`
}

type NotificationTestResult struct {
	ChannelID string    `json:"channelId"`
	Success   bool      `json:"success"`
	ErrorCode string    `json:"errorCode,omitempty"`
	TestedAt  time.Time `json:"testedAt"`
}
