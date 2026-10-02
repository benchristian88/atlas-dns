package adguard

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/adguardcompat"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

const maxResponseBytes = 1 << 20

type Probe struct {
	timeout  time.Duration
	settings interface {
		RuntimeSettings() systemsettings.RuntimeSettings
	}
}

func NewProbe(timeout time.Duration) *Probe {
	return &Probe{timeout: timeout}
}

func (p *Probe) SetRuntimeSettings(provider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}) {
	p.settings = provider
}

func (p *Probe) currentTimeout() time.Duration {
	if p.settings != nil {
		return p.settings.RuntimeSettings().NodeRequestTimeout
	}
	return p.timeout
}

type statusResponse struct {
	Version                      string   `json:"version"`
	Running                      *bool    `json:"running"`
	DNSAddresses                 []string `json:"dns_addresses"`
	DNSPort                      int      `json:"dns_port"`
	ProtectionEnabled            *bool    `json:"protection_enabled"`
	ProtectionDisabledDurationMS *int64   `json:"protection_disabled_duration"`
}

func (p *Probe) Status(ctx context.Context, request domain.NodeProbeRequest) (domain.NodeProbeResult, error) {
	timeout := p.currentTimeout()
	baseURL, err := domain.NormaliseNodeURL(request.BaseURL, request.CertificatePolicy)
	if err != nil {
		return domain.NodeProbeResult{}, err
	}
	endpoint, err := statusEndpoint(baseURL)
	if err != nil {
		return domain.NodeProbeResult{}, domain.Validation("baseUrl", "is not a valid node URL")
	}
	transport, err := p.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return domain.NodeProbeResult{}, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("node status redirects are not allowed")
		},
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.NodeProbeResult{}, fmt.Errorf("create node status request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	started := time.Now()
	response, err := client.Do(httpRequest)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return domain.NodeProbeResult{}, classifyNetworkError(err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NodeProbeResult{}, domain.NewError(domain.ErrorNodeAuth, "the node rejected the supplied credentials")
	default:
		return domain.NodeProbeResult{}, nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, "/control/status", response.StatusCode, response.Header.Get("Content-Type"), "returned an unexpected HTTP status", nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return domain.NodeProbeResult{}, domain.NewError(domain.ErrorNodeResponse, "the node status response could not be read")
	}
	if len(body) > maxResponseBytes {
		return domain.NodeProbeResult{}, domain.NewError(domain.ErrorNodeResponse, "the node status response was too large")
	}
	var status statusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return domain.NodeProbeResult{}, nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, "/control/status", response.StatusCode, response.Header.Get("Content-Type"), "returned invalid JSON", err)
	}
	status.Version = strings.TrimSpace(status.Version)
	if status.Version == "" || len(status.Version) > 128 || status.Running == nil ||
		status.ProtectionEnabled == nil || status.ProtectionDisabledDurationMS == nil ||
		*status.ProtectionDisabledDurationMS < 0 || (*status.ProtectionEnabled && *status.ProtectionDisabledDurationMS > 0) {
		return domain.NodeProbeResult{}, nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, "/control/status", response.StatusCode, response.Header.Get("Content-Type"), "omitted or contradicted required status semantics", nil)
	}
	return domain.NodeProbeResult{
		Version:                      status.Version,
		Compatibility:                VersionCompatibility(status.Version),
		Running:                      *status.Running,
		ProtectionEnabled:            *status.ProtectionEnabled,
		ProtectionDisabledDurationMS: *status.ProtectionDisabledDurationMS,
		LatencyMS:                    latency,
	}, nil
}

func nodeAPIError(kind domain.ErrorKind, method, path string, status int, contentType, problem string, cause error) *domain.Error {
	contentType = strings.TrimSpace(contentType)
	if len(contentType) > 128 {
		contentType = contentType[:128]
	}
	detail := fmt.Sprintf("AdGuard Home node %s %s %s", method, path, problem)
	if status > 0 {
		detail += fmt.Sprintf(" (HTTP %d", status)
		if contentType != "" {
			detail += ", content type " + strconv.Quote(contentType)
		}
		detail += ")"
	}
	if cause != nil {
		detail += ": " + cause.Error()
	}
	return &domain.Error{Kind: kind, Message: detail, Cause: cause}
}

func (p *Probe) transport(policy domain.CertificatePolicy, customCAPEM string) (*http.Transport, error) {
	timeout := p.currentTimeout()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if policy == domain.CertificateCustomCA {
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM([]byte(customCAPEM)) {
			return nil, domain.Validation("customCaPem", "does not contain a valid CA certificate")
		}
		tlsConfig.RootCAs = roots
	}
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       30 * time.Second,
	}, nil
}

func statusEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid base URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/control/status"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func classifyNetworkError(err error) error {
	var certificateUnknown x509.UnknownAuthorityError
	var certificateHost x509.HostnameError
	var certificateInvalid x509.CertificateInvalidError
	if errors.As(err, &certificateUnknown) || errors.As(err, &certificateHost) || errors.As(err, &certificateInvalid) {
		return &domain.Error{Kind: domain.ErrorNodeTLS, Message: "the node TLS certificate could not be verified", Cause: err}
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		var recordHeaderError tls.RecordHeaderError
		if errors.As(urlError.Err, &recordHeaderError) {
			return &domain.Error{Kind: domain.ErrorNodeTLS, Message: "the node did not complete a valid TLS connection", Cause: err}
		}
	}
	return &domain.Error{Kind: domain.ErrorNodeUnreachable, Message: "the AdGuard Home node could not be reached", Cause: err}
}

func VersionCompatibility(version string) domain.Compatibility {
	return ConfigurationCompatibility(version)
}

func OnboardingCompatibility(version string) domain.Compatibility {
	return ConfigurationCompatibility(version)
}

func ConfigurationCompatibility(version string) domain.Compatibility {
	return adguardcompat.Compatibility(version)
}

func IsProvisionallyCompatible(version string) bool {
	return adguardcompat.IsProvisionallyCompatible(version)
}

// SupportsRecentStatistics preserves the managed minimum while allowing later
// products using the legacy control API to inherit exact recent ranges.
func SupportsRecentStatistics(version string) bool {
	return adguardcompat.Generation(version) == adguardcompat.APIGenerationLegacyControl &&
		adguardcompat.Compatibility(version) == domain.CompatibilitySupported
}
