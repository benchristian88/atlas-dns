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

	"github.com/benchristian88/atlas-dns/internal/domain"
)

const maxResponseBytes = 1 << 20

const (
	minimumSupportedMajor = 0
	minimumSupportedMinor = 107
	minimumSupportedPatch = 52
	latestTestedPatch     = 79
)

type Probe struct {
	timeout time.Duration
}

func NewProbe(timeout time.Duration) *Probe {
	return &Probe{timeout: timeout}
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
		Timeout:   p.timeout,
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
			Timeout:   p.timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   p.timeout,
		ResponseHeaderTimeout: p.timeout,
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

func ConfigurationCompatibility(version string) domain.Compatibility {
	major, minor, patch, ok := configurationVersion(version)
	if !ok {
		return domain.CompatibilityUnknown
	}
	if major == minimumSupportedMajor && minor == minimumSupportedMinor && patch >= minimumSupportedPatch {
		return domain.CompatibilitySupported
	}
	if major == minimumSupportedMajor && (minor < minimumSupportedMinor || (minor == minimumSupportedMinor && patch < minimumSupportedPatch)) {
		return domain.CompatibilityUnsupported
	}
	return domain.CompatibilityUnknown
}

// IsProvisionallyCompatible reports versions in the supported API generation
// that are newer than Atlas's latest explicitly contract-tested patch.  Such
// nodes must still pass normal typed endpoint and semantic validation.
func IsProvisionallyCompatible(version string) bool {
	major, minor, patch, ok := configurationVersion(version)
	return ok && major == minimumSupportedMajor && minor == minimumSupportedMinor && patch > latestTestedPatch
}

// IsAdGuard107Generation reports whether version belongs to the API generation
// Atlas can reason about using its explicit patch capability boundaries.
func IsAdGuard107Generation(version string) bool {
	major, minor, _, ok := configurationVersion(version)
	return ok && major == minimumSupportedMajor && minor == minimumSupportedMinor
}

func configurationVersion(version string) (major, minor, patch int, ok bool) {
	value := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(value, ".")
	if len(parts) < 3 {
		return 0, 0, 0, false
	}
	parsedMajor, majorErr := strconv.Atoi(parts[0])
	parsedMinor, minorErr := strconv.Atoi(parts[1])
	parsedPatch, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return 0, 0, 0, false
	}
	return parsedMajor, parsedMinor, parsedPatch, true
}

func supportsEcosia(version string) bool {
	return supportsConfigurationPatch(version, 53)
}

func supportsSchemaV2(version string) bool {
	return supportsEcosia(version)
}

// SupportsRecentStatistics reports whether the tested AdGuard Home API can
// return an exact caller-selected recent window. Earlier supported versions
// expose statistics, but not the range control required for honest 24h/7d/30d
// aggregation.
func SupportsRecentStatistics(version string) bool {
	return supportsConfigurationPatch(version, 72)
}

func supportsConfigurationPatch(version string, minimum int) bool {
	major, minor, patch, ok := configurationVersion(version)
	return ok && major == minimumSupportedMajor && minor == minimumSupportedMinor && patch >= minimum
}
