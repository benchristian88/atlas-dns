package adguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/operations"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
)

type ConfigurationReader struct{ probe *Probe }

const maxConfigurationBody = 4 << 20

func NewConfigurationReader(timeoutProbe *Probe) *ConfigurationReader {
	return &ConfigurationReader{probe: timeoutProbe}
}

type statisticsResponse struct {
	TimeUnits                  string               `json:"time_units"`
	DNSQueries                 int64                `json:"num_dns_queries"`
	BlockedFiltering           int64                `json:"num_blocked_filtering"`
	ReplacedSafeBrowsing       int64                `json:"num_replaced_safebrowsing"`
	ReplacedSafeSearch         int64                `json:"num_replaced_safesearch"`
	ReplacedParental           int64                `json:"num_replaced_parental"`
	AverageProcessingTime      float64              `json:"avg_processing_time"`
	TopQueriedDomains          []map[string]float64 `json:"top_queried_domains"`
	TopBlockedDomains          []map[string]float64 `json:"top_blocked_domains"`
	TopClients                 []map[string]float64 `json:"top_clients"`
	TopUpstreamResponses       []map[string]float64 `json:"top_upstreams_responses"`
	TopUpstreamAverageTime     []map[string]float64 `json:"top_upstreams_avg_time"`
	DNSQueriesSeries           []int64              `json:"dns_queries"`
	BlockedFilteringSeries     []int64              `json:"blocked_filtering"`
	ReplacedSafeBrowsingSeries []int64              `json:"replaced_safebrowsing"`
	ReplacedParentalSeries     []int64              `json:"replaced_parental"`
}

func validateStatisticsResponse(response statisticsResponse) error {
	if response.TimeUnits != "hours" && response.TimeUnits != "days" {
		return domain.NewError(domain.ErrorNodeResponse, "the node statistics response used an invalid time unit")
	}
	if response.DNSQueries < 0 || response.BlockedFiltering < 0 || response.ReplacedSafeBrowsing < 0 ||
		response.ReplacedSafeSearch < 0 || response.ReplacedParental < 0 || response.AverageProcessingTime < 0 ||
		math.IsNaN(response.AverageProcessingTime) || math.IsInf(response.AverageProcessingTime, 0) {
		return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained invalid totals")
	}
	series := [][]int64{response.DNSQueriesSeries, response.BlockedFilteringSeries, response.ReplacedSafeBrowsingSeries, response.ReplacedParentalSeries}
	if len(series[0]) == 0 || len(series[0]) > 1000 {
		return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained an invalid series")
	}
	for _, values := range series {
		if len(values) != len(series[0]) {
			return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained mismatched series")
		}
		for _, value := range values {
			if value < 0 {
				return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained a negative series value")
			}
		}
	}
	for _, ranked := range [][]map[string]float64{response.TopQueriedDomains, response.TopBlockedDomains, response.TopClients, response.TopUpstreamResponses, response.TopUpstreamAverageTime} {
		if len(ranked) > 100 {
			return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained too many ranked values")
		}
		for _, item := range ranked {
			if len(item) != 1 {
				return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained an invalid ranked value")
			}
			for key, value := range item {
				if strings.TrimSpace(key) == "" || len(key) > 512 || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
					return domain.NewError(domain.ErrorNodeResponse, "the node statistics response contained an invalid ranked value")
				}
			}
		}
	}
	return nil
}

func flattenRanked(values []map[string]float64) []telemetry.RankedValue {
	result := make([]telemetry.RankedValue, 0, len(values))
	for _, item := range values {
		for key, value := range item {
			result = append(result, telemetry.RankedValue{Key: strings.TrimSpace(key), Value: value})
		}
	}
	return result
}

// ReadStatistics reads an exact recent statistics window and rejects malformed
// or unbounded node data before it reaches durable storage.
func (r *ConfigurationReader) ReadStatistics(ctx context.Context, request domain.NodeProbeRequest, recent time.Duration) (telemetry.SourceSnapshot, error) {
	if recent <= 0 || recent%time.Hour != 0 {
		return telemetry.SourceSnapshot{}, domain.Validation("recent", "must be a positive whole-hour duration")
	}
	query := url.Values{"recent": []string{strconv.FormatInt(recent.Milliseconds(), 10)}}
	var response statisticsResponse
	if err := r.getOperationalResource(ctx, request, "/control/stats", query, &response); err != nil {
		return telemetry.SourceSnapshot{}, err
	}
	if err := validateStatisticsResponse(response); err != nil {
		return telemetry.SourceSnapshot{}, err
	}
	return telemetry.SourceSnapshot{
		TimeUnit: response.TimeUnits, DNSQueries: response.DNSQueries,
		BlockedFiltering: response.BlockedFiltering, ReplacedSafeBrowsing: response.ReplacedSafeBrowsing,
		ReplacedSafeSearch: response.ReplacedSafeSearch, ReplacedParental: response.ReplacedParental,
		AverageProcessingSeconds: response.AverageProcessingTime,
		TopQueriedDomains:        flattenRanked(response.TopQueriedDomains), TopBlockedDomains: flattenRanked(response.TopBlockedDomains),
		TopClients: flattenRanked(response.TopClients), TopUpstreamResponses: flattenRanked(response.TopUpstreamResponses),
		TopUpstreamAverageSeconds: flattenRanked(response.TopUpstreamAverageTime), DNSQueriesSeries: response.DNSQueriesSeries,
		BlockedFilteringSeries: response.BlockedFilteringSeries, ReplacedSafeBrowsingSeries: response.ReplacedSafeBrowsingSeries,
		ReplacedParentalSeries: response.ReplacedParentalSeries,
	}, nil
}

// ReadStatisticsConfig returns the node-local retention boundary used to
// select exact ranges.  AdGuard Home rejects a recent window greater than this
// interval, so callers must inspect it before requesting statistics.
func (r *ConfigurationReader) ReadStatisticsConfig(ctx context.Context, request domain.NodeProbeRequest) (telemetry.SourceConfig, error) {
	var response policyResponse
	if err := r.get(ctx, request, "/control/stats/config", &response); err != nil {
		return telemetry.SourceConfig{}, err
	}
	const (
		minimum = int64(time.Hour / time.Millisecond)
		maximum = int64(365 * 24 * time.Hour / time.Millisecond)
	)
	if response.IntervalMillis < 0 || response.IntervalMillis > maximum ||
		(response.IntervalMillis != 0 && response.IntervalMillis < minimum) ||
		(response.Enabled && response.IntervalMillis == 0) {
		return telemetry.SourceConfig{}, domain.NewError(domain.ErrorNodeResponse, "the node statistics configuration used an invalid retention interval")
	}
	return telemetry.SourceConfig{Enabled: response.Enabled, Retention: time.Duration(response.IntervalMillis) * time.Millisecond}, nil
}

type dnsInfoResponse struct {
	UpstreamDNS             []string   `json:"upstream_dns"`
	BootstrapDNS            []string   `json:"bootstrap_dns"`
	FallbackDNS             []string   `json:"fallback_dns"`
	PrivateReverseDNS       []string   `json:"local_ptr_upstreams"`
	ProtectionEnabled       *bool      `json:"protection_enabled"`
	ProtectionDisabledUntil *time.Time `json:"protection_disabled_until"`
	RateLimit               int        `json:"ratelimit"`
	RateLimitIPv4           int        `json:"ratelimit_subnet_len_ipv4"`
	RateLimitIPv6           int        `json:"ratelimit_subnet_len_ipv6"`
	RateLimitAllowlist      []string   `json:"ratelimit_whitelist"`
	BlockingMode            string     `json:"blocking_mode"`
	BlockingIPv4            string     `json:"blocking_ipv4"`
	BlockingIPv6            string     `json:"blocking_ipv6"`
	BlockedResponseTTL      int        `json:"blocked_response_ttl"`
	EDNSClientSubnet        bool       `json:"edns_cs_enabled"`
	EDNSUseCustom           bool       `json:"edns_cs_use_custom"`
	EDNSCustomIP            string     `json:"edns_cs_custom_ip"`
	DisableIPv6             bool       `json:"disable_ipv6"`
	DNSSECEnabled           bool       `json:"dnssec_enabled"`
	CacheSize               int        `json:"cache_size"`
	CacheEnabled            *bool      `json:"cache_enabled"`
	CacheTTLMin             int        `json:"cache_ttl_min"`
	CacheTTLMax             int        `json:"cache_ttl_max"`
	CacheOptimistic         bool       `json:"cache_optimistic"`
	UpstreamMode            string     `json:"upstream_mode"`
	UsePrivateReverse       bool       `json:"use_private_ptr_resolvers"`
	ResolveClients          bool       `json:"resolve_clients"`
	UpstreamTimeout         *int       `json:"upstream_timeout"`
}

type filterListResponse struct {
	ID          int64  `json:"id"`
	Enabled     bool   `json:"enabled"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	RulesCount  int64  `json:"rules_count"`
	LastUpdated string `json:"last_updated"`
	Whitelist   bool   `json:"whitelist"`
}

type filterStatusResponse struct {
	FilteringEnabled *bool                `json:"filtering_enabled"`
	Enabled          *bool                `json:"enabled"`
	Interval         int                  `json:"interval"`
	Filters          []filterListResponse `json:"filters"`
	UserRules        []string             `json:"user_rules"`
	WhitelistFilters []filterListResponse `json:"whitelist_filters"`
}

type filterCheckHostResponse struct {
	Reason string `json:"reason"`
	Rules  []struct {
		Text         string `json:"text"`
		FilterListID int64  `json:"filter_list_id"`
	} `json:"rules"`
	Rule        string   `json:"rule"`
	FilterID    int64    `json:"filter_id"`
	ServiceName string   `json:"service_name"`
	CNAME       string   `json:"cname"`
	IPAddresses []string `json:"ip_addrs"`
}

type clientsResponse struct {
	Clients []clientResponse `json:"clients"`
}

type clientResponse struct {
	Name                     string             `json:"name"`
	IDs                      []string           `json:"ids"`
	UseGlobalSettings        bool               `json:"use_global_settings"`
	FilteringEnabled         bool               `json:"filtering_enabled"`
	ParentalEnabled          bool               `json:"parental_enabled"`
	SafeBrowsingEnabled      bool               `json:"safebrowsing_enabled"`
	SafeSearch               safeSearchResponse `json:"safe_search"`
	UseGlobalBlockedServices bool               `json:"use_global_blocked_services"`
	BlockedServices          []string           `json:"blocked_services"`
	BlockedServicesSchedule  scheduleResponse   `json:"blocked_services_schedule"`
	Upstreams                []string           `json:"upstreams"`
	UpstreamsCacheEnabled    bool               `json:"upstreams_cache_enabled"`
	UpstreamsCacheSize       int                `json:"upstreams_cache_size"`
	Tags                     []string           `json:"tags"`
	IgnoreQueryLog           bool               `json:"ignore_querylog"`
	IgnoreStatistics         bool               `json:"ignore_statistics"`
}

type rewriteResponse struct {
	Domain  string `json:"domain"`
	Answer  string `json:"answer"`
	Enabled *bool  `json:"enabled"`
}
type enabledResponse struct {
	Enabled bool `json:"enabled"`
}
type rewriteSettingsResponse struct {
	Enabled bool `json:"enabled"`
}
type safeSearchResponse struct {
	Enabled    bool `json:"enabled"`
	Bing       bool `json:"bing"`
	DuckDuckGo bool `json:"duckduckgo"`
	Ecosia     bool `json:"ecosia"`
	Google     bool `json:"google"`
	Pixabay    bool `json:"pixabay"`
	Yandex     bool `json:"yandex"`
	YouTube    bool `json:"youtube"`
}
type dayRangeResponse struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}
type scheduleResponse struct {
	TimeZone string            `json:"time_zone"`
	Sun      *dayRangeResponse `json:"sun"`
	Mon      *dayRangeResponse `json:"mon"`
	Tue      *dayRangeResponse `json:"tue"`
	Wed      *dayRangeResponse `json:"wed"`
	Thu      *dayRangeResponse `json:"thu"`
	Fri      *dayRangeResponse `json:"fri"`
	Sat      *dayRangeResponse `json:"sat"`
}
type blockedServicesResponse struct {
	Schedule scheduleResponse `json:"schedule"`
	IDs      []string         `json:"ids"`
}
type blockedServicesCatalogueResponse struct {
	BlockedServices []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		GroupID string `json:"group_id"`
	} `json:"blocked_services"`
	Groups []struct {
		ID string `json:"id"`
	} `json:"groups"`
}
type policyResponse struct {
	Enabled           bool     `json:"enabled"`
	IntervalMillis    int64    `json:"interval"`
	AnonymizeClientIP bool     `json:"anonymize_client_ip"`
	Ignored           []string `json:"ignored"`
	IgnoredEnabled    *bool    `json:"ignored_enabled"`
}
type tlsStatusResponse struct {
	Enabled         bool     `json:"enabled"`
	ServerName      string   `json:"server_name"`
	ForceHTTPS      bool     `json:"force_https"`
	HTTPSPort       int      `json:"port_https"`
	DNSOverTLSPort  int      `json:"port_dns_over_tls"`
	DNSOverQUICPort int      `json:"port_dns_over_quic"`
	ServePlainDNS   bool     `json:"serve_plain_dns"`
	ValidCert       bool     `json:"valid_cert"`
	ValidChain      bool     `json:"valid_chain"`
	ValidKey        bool     `json:"valid_key"`
	ValidPair       bool     `json:"valid_pair"`
	Subject         string   `json:"subject"`
	Issuer          string   `json:"issuer"`
	NotBefore       string   `json:"not_before"`
	NotAfter        string   `json:"not_after"`
	DNSNames        []string `json:"dns_names"`
	Warning         string   `json:"warning_validation"`
}
type dhcpV4Response struct {
	Gateway       string `json:"gateway_ip"`
	SubnetMask    string `json:"subnet_mask"`
	RangeStart    string `json:"range_start"`
	RangeEnd      string `json:"range_end"`
	LeaseDuration int64  `json:"lease_duration"`
}
type dhcpV6Response struct {
	RangeStart    string `json:"range_start"`
	LeaseDuration int64  `json:"lease_duration"`
}
type dhcpLeaseResponse struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Expires  string `json:"expires"`
}
type dhcpStatusResponse struct {
	Enabled       bool                `json:"enabled"`
	InterfaceName string              `json:"interface_name"`
	V4            dhcpV4Response      `json:"v4"`
	V6            dhcpV6Response      `json:"v6"`
	Leases        []dhcpLeaseResponse `json:"leases"`
	StaticLeases  []dhcpLeaseResponse `json:"static_leases"`
}

type dhcpInterfaceResponse struct {
	Name            string   `json:"name"`
	HardwareAddress string   `json:"hardware_address"`
	IPv4Addresses   []string `json:"ipv4_addresses"`
	IPv6Addresses   []string `json:"ipv6_addresses"`
	GatewayIP       string   `json:"gateway_ip"`
	Flags           string   `json:"flags"`
}

type dhcpCheckResponseValue struct {
	Status string `json:"found"`
	Error  string `json:"error"`
}

type dhcpStaticIPResponse struct {
	Status string `json:"static"`
	IP     string `json:"ip"`
}

type dhcpActiveCheckResponse struct {
	V4 struct {
		OtherServer dhcpCheckResponseValue `json:"other_server"`
		StaticIP    dhcpStaticIPResponse   `json:"static_ip"`
	} `json:"v4"`
	V6 struct {
		OtherServer dhcpCheckResponseValue `json:"other_server"`
	} `json:"v6"`
}

func (r *ConfigurationReader) ReadConfiguration(ctx context.Context, request domain.NodeProbeRequest, version string) (configuration.Document, inventory.CapabilityProfile, error) {
	profile := inventory.CapabilityProfile{ProductVersion: version, Compatibility: string(ConfigurationCompatibility(version)), SchemaVersion: configuration.SchemaVersion, Features: map[string]bool{"dns": false, "cache_toggle": false, "upstream_timeout": false, "test_upstream_dns": false, "cache_clear": false, "filtering": false, "test_host_filtering": false, "test_host_filtering_context": false, "filter_interval_arbitrary": false, "clients": false, "rewrites": false, "rewrite_toggle": false, "blocked_services": false, "safety": false, "safe_search_ecosia": supportsEcosia(version), "query_log": false, "querylog_clear": false, "statistics": false, "statistics_exact_range": SupportsRecentStatistics(version), "stats_reset": false, "ignored_lists_toggle": false, "tls": false, "dhcp": false}, Warnings: []string{}}
	if ConfigurationCompatibility(version) != domain.CompatibilitySupported {
		profile.Warnings = append(profile.Warnings, "This AdGuard Home version is outside the tested configuration inventory range.")
		return configuration.Document{}, profile, domain.NewError(domain.ErrorCapability, "the node version is outside the supported AdGuard Home API generation")
	}
	if IsProvisionallyCompatible(version) {
		profile.Warnings = append(profile.Warnings, "This newer AdGuard Home 0.107 patch is provisionally compatible; Atlas validated the APIs it uses, but this patch has not been explicitly release-tested.")
	}
	// These destructive endpoints predate the supported v0.107.52 floor and do
	// not depend on schema-v2 policy inventory.
	profile.Features["querylog_clear"] = true
	profile.Features["stats_reset"] = true
	var status statusResponse
	if err := r.get(ctx, request, "/control/status", &status); err != nil {
		profile.Warnings = append(profile.Warnings, "DNS listener configuration could not be read.")
		return configuration.Document{}, profile, err
	}
	if err := validateListenerStatus(status); err != nil {
		profile.Warnings = append(profile.Warnings, "DNS listener configuration could not be read.")
		return configuration.Document{}, profile, err
	}
	var dns dnsInfoResponse
	if err := r.get(ctx, request, "/control/dns_info", &dns); err != nil {
		profile.Warnings = append(profile.Warnings, "DNS configuration could not be read.")
		return configuration.Document{}, profile, err
	}
	if err := validateDNSProtection(dns); err != nil {
		profile.Warnings = append(profile.Warnings, "DNS protection state could not be validated.")
		return configuration.Document{}, profile, err
	}
	profile.Features["dns"] = true
	profile.Features["test_upstream_dns"] = true
	profile.Features["cache_clear"] = true
	profile.Features["cache_toggle"] = dns.CacheEnabled != nil
	profile.Features["upstream_timeout"] = dns.UpstreamTimeout != nil
	var filtering filterStatusResponse
	if err := r.get(ctx, request, "/control/filtering/status", &filtering); err != nil {
		profile.Warnings = append(profile.Warnings, "Filtering configuration could not be read.")
		return configuration.Document{}, profile, err
	}
	profile.Features["filtering"] = true
	profile.Features["test_host_filtering"] = true
	profile.Features["test_host_filtering_context"] = supportsConfigurationPatch(version, 58)
	profile.Features["filter_interval_arbitrary"] = supportsConfigurationPatch(version, 78)
	if !supportsSchemaV2(version) {
		profile.SchemaVersion = configuration.LegacySchemaVersion
		profile.Warnings = append(profile.Warnings, "AdGuard Home v0.107.53 or later in the supported 0.107 API generation is required for schema-v2 configuration management; legacy schema-v1 inventory remains available.")
		document := configuration.ProjectDocument(configurationDocument(version, status, dns, filtering), configuration.LegacySchemaVersion)
		document.Unsupported = []configuration.Unsupported{
			{Section: "services", Reason: "blocked services and safety services require schema-v2 inventory"},
			{Section: "tls_dhcp", Reason: "TLS and DHCP require schema-v2 inventory"},
		}
		return configuration.Canonicalise(document), profile, nil
	}
	var clients clientsResponse
	if err := r.get(ctx, request, "/control/clients", &clients); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["clients"] = true
	var rewrites []rewriteResponse
	if err := r.get(ctx, request, "/control/rewrite/list", &rewrites); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["rewrites"] = true
	rewriteSettings := rewriteSettingsResponse{Enabled: true}
	if supportsConfigurationPatch(version, 68) {
		if err := r.get(ctx, request, "/control/rewrite/settings", &rewriteSettings); err != nil {
			return configuration.Document{}, profile, err
		}
		profile.Features["rewrite_toggle"] = true
	}
	var blocked blockedServicesResponse
	if err := r.get(ctx, request, "/control/blocked_services/get", &blocked); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["blocked_services"] = true
	var safeBrowsing, parental enabledResponse
	var safeSearch safeSearchResponse
	if err := r.get(ctx, request, "/control/safebrowsing/status", &safeBrowsing); err != nil {
		return configuration.Document{}, profile, err
	}
	if err := r.get(ctx, request, "/control/parental/status", &parental); err != nil {
		return configuration.Document{}, profile, err
	}
	if err := r.get(ctx, request, "/control/safesearch/status", &safeSearch); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["safety"] = true
	var queryLog, statistics policyResponse
	if err := r.get(ctx, request, "/control/querylog/config", &queryLog); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["query_log"] = true
	if err := r.get(ctx, request, "/control/stats/config", &statistics); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["statistics"] = true
	profile.Features["ignored_lists_toggle"] = queryLog.IgnoredEnabled != nil && statistics.IgnoredEnabled != nil
	var tls tlsStatusResponse
	if err := r.get(ctx, request, "/control/tls/status", &tls); err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["tls"] = true
	var dhcp dhcpStatusResponse
	dhcpSupported, err := r.getOptional(ctx, request, "/control/dhcp/status", &dhcp)
	if err != nil {
		return configuration.Document{}, profile, err
	}
	profile.Features["dhcp"] = dhcpSupported
	if !dhcpSupported {
		profile.Warnings = append(profile.Warnings, "DHCP is unavailable on this node and remains unmanaged.")
	}
	document := configurationDocument(version, status, dns, filtering)
	populateBroaderDocument(&document, clients, rewriteSettings, rewrites, blocked, safeBrowsing, parental, safeSearch, queryLog, statistics, tls, dhcp, dhcpSupported)
	return configuration.Canonicalise(document), profile, nil
}

func (r *ConfigurationReader) ReadBlockedServicesCatalogue(ctx context.Context, request domain.NodeProbeRequest, version string) (inventory.NodeBlockedServicesCatalogue, error) {
	if ConfigurationCompatibility(version) != domain.CompatibilitySupported {
		return inventory.NodeBlockedServicesCatalogue{}, domain.NewError(domain.ErrorCapability, "the node version is not supported for blocked-services catalogue inventory")
	}
	var response blockedServicesCatalogueResponse
	if err := r.get(ctx, request, "/control/blocked_services/all", &response); err != nil {
		return inventory.NodeBlockedServicesCatalogue{}, err
	}
	result := inventory.NodeBlockedServicesCatalogue{
		Services: make([]inventory.BlockedServiceMetadata, 0, len(response.BlockedServices)),
		Groups:   make([]inventory.BlockedServiceGroup, 0, len(response.Groups)),
	}
	seen := map[string]bool{}
	for _, service := range response.BlockedServices {
		service.ID, service.Name, service.GroupID = strings.TrimSpace(service.ID), strings.TrimSpace(service.Name), strings.TrimSpace(service.GroupID)
		if service.ID == "" || service.Name == "" || len(service.ID) > 200 || len(service.Name) > 500 || len(service.GroupID) > 200 || seen[service.ID] {
			return inventory.NodeBlockedServicesCatalogue{}, domain.NewError(domain.ErrorNodeResponse, "the node returned invalid blocked-services catalogue metadata")
		}
		seen[service.ID] = true
		result.Services = append(result.Services, inventory.BlockedServiceMetadata{ID: service.ID, Name: service.Name, GroupID: service.GroupID})
	}
	seenGroups := map[string]bool{}
	for _, group := range response.Groups {
		id := strings.TrimSpace(group.ID)
		if id == "" || len(id) > 200 || seenGroups[id] {
			return inventory.NodeBlockedServicesCatalogue{}, domain.NewError(domain.ErrorNodeResponse, "the node returned invalid blocked-services group metadata")
		}
		seenGroups[id] = true
		result.Groups = append(result.Groups, inventory.BlockedServiceGroup{ID: id})
	}
	return result, nil
}

func (r *ConfigurationReader) ReadDHCPInterfaces(ctx context.Context, request domain.NodeProbeRequest) ([]inventory.DHCPInterface, error) {
	var response map[string]dhcpInterfaceResponse
	supported, err := r.getOptional(ctx, request, "/control/dhcp/interfaces", &response)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, domain.NewError(domain.ErrorCapability, "DHCP interface discovery is not supported by this node")
	}
	interfaces := make([]inventory.DHCPInterface, 0, len(response))
	for key, item := range response {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" || len(name) > 128 {
			return nil, domain.NewError(domain.ErrorNodeResponse, "the node returned invalid DHCP interface metadata")
		}
		flags := []string{}
		for _, flag := range strings.Split(item.Flags, "|") {
			if flag = strings.TrimSpace(flag); flag != "" {
				flags = append(flags, flag)
			}
		}
		interfaces = append(interfaces, inventory.DHCPInterface{
			Name: name, HardwareAddress: strings.TrimSpace(item.HardwareAddress),
			IPv4Addresses: item.IPv4Addresses, IPv6Addresses: item.IPv6Addresses,
			GatewayIP: strings.TrimSpace(item.GatewayIP), Flags: flags,
		})
	}
	return interfaces, nil
}

func (r *ConfigurationReader) FindActiveDHCP(ctx context.Context, request domain.NodeProbeRequest, interfaceName string) (inventory.DHCPActiveCheck, error) {
	var response dhcpActiveCheckResponse
	if err := r.postResource(ctx, request, "/control/dhcp/find_active_dhcp", map[string]any{"interface": interfaceName}, &response); err != nil {
		return inventory.DHCPActiveCheck{}, err
	}
	v4, err := dhcpCheckValue(response.V4.OtherServer.Status)
	if err != nil {
		return inventory.DHCPActiveCheck{}, err
	}
	v6, err := dhcpCheckValue(response.V6.OtherServer.Status)
	if err != nil {
		return inventory.DHCPActiveCheck{}, err
	}
	staticStatus := strings.TrimSpace(response.V4.StaticIP.Status)
	if staticStatus == "" {
		staticStatus = "unavailable"
	}
	if staticStatus != "yes" && staticStatus != "no" && staticStatus != "error" && staticStatus != "unavailable" {
		return inventory.DHCPActiveCheck{}, domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid static-IP check result")
	}
	return inventory.DHCPActiveCheck{
		IPv4OtherServer: v4,
		IPv4StaticIP:    inventory.DHCPStaticIPCheck{Status: staticStatus, IP: strings.TrimSpace(response.V4.StaticIP.IP)},
		IPv6OtherServer: v6,
	}, nil
}

func dhcpCheckValue(status string) (inventory.DHCPCheckValue, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return inventory.DHCPCheckValue{Status: "unavailable"}, nil
	}
	if status != "yes" && status != "no" && status != "error" {
		return inventory.DHCPCheckValue{}, domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid active DHCP check result")
	}
	value := inventory.DHCPCheckValue{Status: status}
	if status == "error" {
		value.Message = "The node could not determine whether another DHCP server is active."
	}
	return value, nil
}

func (r *ConfigurationReader) ReadBlocklists(ctx context.Context, request domain.NodeProbeRequest, version string) ([]inventory.FilterListMetadata, error) {
	return r.readFilterLists(ctx, request, version, false)
}

func (r *ConfigurationReader) ReadAllowlists(ctx context.Context, request domain.NodeProbeRequest, version string) ([]inventory.FilterListMetadata, error) {
	return r.readFilterLists(ctx, request, version, true)
}

func (r *ConfigurationReader) readFilterLists(ctx context.Context, request domain.NodeProbeRequest, version string, whitelist bool) ([]inventory.FilterListMetadata, error) {
	if ConfigurationCompatibility(version) != domain.CompatibilitySupported {
		return nil, domain.NewError(domain.ErrorCapability, "the node version is not supported for filter-list presentation")
	}
	var response filterStatusResponse
	if err := r.get(ctx, request, "/control/filtering/status", &response); err != nil {
		return nil, err
	}
	filters := append([]filterListResponse(nil), response.Filters...)
	for _, filter := range response.WhitelistFilters {
		filter.Whitelist = true
		filters = append(filters, filter)
	}
	result := make([]inventory.FilterListMetadata, 0, len(filters))
	for _, filter := range filters {
		if filter.Whitelist != whitelist {
			continue
		}
		item := inventory.FilterListMetadata{ID: filter.ID, URL: filter.URL, Name: filter.Name, Enabled: filter.Enabled, RulesCount: filter.RulesCount}
		if strings.TrimSpace(filter.LastUpdated) != "" {
			updated, err := time.Parse(time.RFC3339, filter.LastUpdated)
			if err != nil {
				return nil, domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid filter-list update time")
			}
			updated = updated.UTC()
			item.LastUpdated = &updated
		}
		result = append(result, item)
	}
	return result, nil
}

func validateListenerStatus(status statusResponse) error {
	if status.DNSPort < 1 || status.DNSPort > 65535 || len(status.DNSAddresses) == 0 ||
		status.ProtectionEnabled == nil || status.ProtectionDisabledDurationMS == nil ||
		*status.ProtectionDisabledDurationMS < 0 || (*status.ProtectionEnabled && *status.ProtectionDisabledDurationMS > 0) {
		return domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid DNS listener configuration")
	}
	for _, address := range status.DNSAddresses {
		if _, err := netip.ParseAddr(strings.TrimSpace(address)); err != nil {
			return domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid DNS listener configuration")
		}
	}
	return nil
}

func validateDNSProtection(dns dnsInfoResponse) error {
	if dns.ProtectionEnabled == nil || (*dns.ProtectionEnabled && dns.ProtectionDisabledUntil != nil) {
		return domain.NewError(domain.ErrorNodeResponse, "the node returned an invalid DNS protection state")
	}
	return nil
}

func configurationDocument(version string, status statusResponse, dns dnsInfoResponse, filtering filterStatusResponse) configuration.Document {
	filterURLs := make([]string, 0, len(filtering.Filters))
	whitelistURLs := make([]string, 0, len(filtering.WhitelistFilters))
	for _, filter := range filtering.Filters {
		if filter.Enabled {
			if filter.Whitelist {
				whitelistURLs = append(whitelistURLs, filter.URL)
			} else {
				filterURLs = append(filterURLs, filter.URL)
			}
		}
	}
	for _, filter := range filtering.WhitelistFilters {
		if filter.Enabled {
			whitelistURLs = append(whitelistURLs, filter.URL)
		}
	}
	enabled := false
	if filtering.Enabled != nil {
		enabled = *filtering.Enabled
	} else if filtering.FilteringEnabled != nil {
		enabled = *filtering.FilteringEnabled
	}
	return configuration.Document{
		SchemaVersion: configuration.SchemaVersion,
		Shared: configuration.Shared{DNS: configuration.DNS{
			UpstreamDNS: dns.UpstreamDNS, BootstrapDNS: dns.BootstrapDNS, FallbackDNS: dns.FallbackDNS, PrivateReverseDNS: dns.PrivateReverseDNS,
			ProtectionEnabled: valueOrDefault(dns.ProtectionEnabled, false), RateLimit: dns.RateLimit, RateLimitIPv4: dns.RateLimitIPv4, RateLimitIPv6: dns.RateLimitIPv6,
			RateLimitAllowlist: dns.RateLimitAllowlist, BlockingMode: dns.BlockingMode, BlockingIPv4: dns.BlockingIPv4, BlockingIPv6: dns.BlockingIPv6,
			BlockedResponseTTL: dns.BlockedResponseTTL, EDNSClientSubnet: dns.EDNSClientSubnet, EDNSUseCustom: dns.EDNSUseCustom,
			EDNSCustomIP: dns.EDNSCustomIP, DisableIPv6: dns.DisableIPv6, DNSSECEnabled: dns.DNSSECEnabled, CacheSize: dns.CacheSize, CacheEnabled: valueOrDefault(dns.CacheEnabled, dns.CacheSize > 0),
			CacheTTLMin: dns.CacheTTLMin, CacheTTLMax: dns.CacheTTLMax, CacheOptimistic: dns.CacheOptimistic, UpstreamMode: dns.UpstreamMode,
			UsePrivateReverse: dns.UsePrivateReverse, ResolveClients: dns.ResolveClients, UpstreamTimeout: valueOrDefault(dns.UpstreamTimeout, 0),
		}, Filtering: configuration.Filtering{Enabled: enabled, UpdateInterval: filtering.Interval, FilterURLs: filterURLs, WhitelistURLs: whitelistURLs, UserRules: filtering.UserRules}},
		NodeSpecific: configuration.NodeSpecific{BindHosts: status.DNSAddresses, DNSPort: status.DNSPort},
		ObservedOnly: configuration.ObservedOnly{ProductVersion: version, ProtectionDisabledUntil: protectionDisabledUntil(dns.ProtectionDisabledUntil)},
		Unsupported:  []configuration.Unsupported{{Section: "tls_mutation", Reason: "TLS is inventory-only until controller secret references are implemented"}},
	}
}

func protectionDisabledUntil(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func valueOrDefault[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func populateBroaderDocument(document *configuration.Document, clients clientsResponse, rewriteSettings rewriteSettingsResponse, rewrites []rewriteResponse, blocked blockedServicesResponse, safeBrowsing, parental enabledResponse, safeSearch safeSearchResponse, queryLog, statistics policyResponse, tls tlsStatusResponse, dhcp dhcpStatusResponse, dhcpSupported bool) {
	document.Shared.Clients = make([]configuration.PersistentClient, 0, len(clients.Clients))
	for _, client := range clients.Clients {
		document.Shared.Clients = append(document.Shared.Clients, configuration.PersistentClient{Name: client.Name, IDs: client.IDs, UseGlobalSettings: client.UseGlobalSettings, FilteringEnabled: client.FilteringEnabled, ParentalEnabled: client.ParentalEnabled, SafeBrowsingEnabled: client.SafeBrowsingEnabled, SafeSearch: safeSearchModel(client.SafeSearch), UseGlobalBlockedServices: client.UseGlobalBlockedServices, BlockedServices: client.BlockedServices, BlockedServicesSchedule: scheduleModel(client.BlockedServicesSchedule), Upstreams: client.Upstreams, UpstreamsCacheEnabled: client.UpstreamsCacheEnabled, UpstreamsCacheSize: client.UpstreamsCacheSize, Tags: client.Tags, IgnoreQueryLog: client.IgnoreQueryLog, IgnoreStatistics: client.IgnoreStatistics})
	}
	document.Shared.RewritesEnabled = rewriteSettings.Enabled
	document.Shared.Rewrites = make([]configuration.Rewrite, 0, len(rewrites))
	for _, rewrite := range rewrites {
		document.Shared.Rewrites = append(document.Shared.Rewrites, configuration.Rewrite{Domain: rewrite.Domain, Answer: rewrite.Answer, Enabled: valueOrDefault(rewrite.Enabled, true)})
	}
	document.Shared.Services = configuration.Services{BlockedServiceIDs: blocked.IDs, BlockedSchedule: scheduleModel(blocked.Schedule), SafeBrowsing: safeBrowsing.Enabled, ParentalControl: parental.Enabled, SafeSearch: safeSearchModel(safeSearch)}
	document.Shared.QueryLog = configuration.QueryLogPolicy{Enabled: queryLog.Enabled, IntervalMillis: queryLog.IntervalMillis, AnonymizeClientIP: queryLog.AnonymizeClientIP, Ignored: queryLog.Ignored, IgnoredEnabled: valueOrDefault(queryLog.IgnoredEnabled, true)}
	document.Shared.Statistics = configuration.StatisticsPolicy{Enabled: statistics.Enabled, IntervalMillis: statistics.IntervalMillis, Ignored: statistics.Ignored, IgnoredEnabled: valueOrDefault(statistics.IgnoredEnabled, true)}
	document.ObservedOnly.TLS = configuration.TLSStatus{Enabled: tls.Enabled, ServerName: tls.ServerName, ForceHTTPS: tls.ForceHTTPS, HTTPSPort: tls.HTTPSPort, DNSOverTLSPort: tls.DNSOverTLSPort, DNSOverQUICPort: tls.DNSOverQUICPort, ServePlainDNS: tls.ServePlainDNS, ValidCertificate: tls.ValidCert, ValidChain: tls.ValidChain, ValidKey: tls.ValidKey, ValidPair: tls.ValidPair, Subject: tls.Subject, Issuer: tls.Issuer, NotBefore: tls.NotBefore, NotAfter: tls.NotAfter, DNSNames: tls.DNSNames, Warning: tls.Warning}
	if !dhcpSupported {
		document.Unsupported = append(document.Unsupported, configuration.Unsupported{Section: "dhcp", Reason: "the node reports that DHCP is unavailable"})
		return
	}
	staticLeases := make([]configuration.DHCPStaticLease, 0, len(dhcp.StaticLeases))
	for _, lease := range dhcp.StaticLeases {
		staticLeases = append(staticLeases, configuration.DHCPStaticLease{MAC: lease.MAC, IP: lease.IP, Hostname: lease.Hostname})
	}
	document.NodeSpecific.DHCP = &configuration.DHCPConfig{Enabled: dhcp.Enabled, InterfaceName: dhcp.InterfaceName, IPv4: configuration.DHCPIPv4{Gateway: dhcp.V4.Gateway, SubnetMask: dhcp.V4.SubnetMask, RangeStart: dhcp.V4.RangeStart, RangeEnd: dhcp.V4.RangeEnd, LeaseDuration: dhcp.V4.LeaseDuration}, IPv6: configuration.DHCPIPv6{RangeStart: dhcp.V6.RangeStart, LeaseDuration: dhcp.V6.LeaseDuration}, StaticLeases: staticLeases}
	document.ObservedOnly.DHCPLeases = make([]configuration.DHCPLease, 0, len(dhcp.Leases))
	for _, lease := range dhcp.Leases {
		document.ObservedOnly.DHCPLeases = append(document.ObservedOnly.DHCPLeases, configuration.DHCPLease{MAC: lease.MAC, IP: lease.IP, Hostname: lease.Hostname, ExpiresAt: lease.Expires})
	}
}

func safeSearchModel(value safeSearchResponse) configuration.SafeSearch {
	return configuration.SafeSearch{Enabled: value.Enabled, Bing: value.Bing, DuckDuckGo: value.DuckDuckGo, Ecosia: value.Ecosia, Google: value.Google, Pixabay: value.Pixabay, Yandex: value.Yandex, YouTube: value.YouTube}
}

func scheduleModel(value scheduleResponse) configuration.Schedule {
	days := map[string]configuration.DayRange{}
	for day, period := range map[string]*dayRangeResponse{"sun": value.Sun, "mon": value.Mon, "tue": value.Tue, "wed": value.Wed, "thu": value.Thu, "fri": value.Fri, "sat": value.Sat} {
		if period != nil {
			days[day] = configuration.DayRange{Start: period.Start, End: period.End}
		}
	}
	return configuration.Schedule{TimeZone: value.TimeZone, Days: days}
}

// ApplyConfiguration mutates the fields owned by the document's schema through
// supported AdGuard Home HTTP APIs. Listener addresses and ports have no
// supported writer; callers must preflight them before invoking this method.
func (r *ConfigurationReader) ApplyConfiguration(ctx context.Context, request domain.NodeProbeRequest, desired configuration.Document) error {
	desired = configuration.Canonicalise(desired)
	var currentFilters filterStatusResponse
	if err := r.get(ctx, request, "/control/filtering/status", &currentFilters); err != nil {
		return err
	}
	var currentDNS dnsInfoResponse
	if desired.SchemaVersion >= configuration.SchemaVersion {
		if err := r.get(ctx, request, "/control/dns_info", &currentDNS); err != nil {
			return err
		}
	}
	if err := r.post(ctx, request, "/control/filtering/config", map[string]any{
		"enabled": desired.Shared.Filtering.Enabled, "interval": desired.Shared.Filtering.UpdateInterval,
	}); err != nil {
		return err
	}
	if err := r.reconcileFilterURLs(ctx, request, currentFilters, desired.Shared.Filtering.FilterURLs, false); err != nil {
		return err
	}
	if desired.SchemaVersion >= configuration.SchemaVersion {
		if err := r.reconcileFilterURLs(ctx, request, currentFilters, desired.Shared.Filtering.WhitelistURLs, true); err != nil {
			return err
		}
	}
	if err := r.post(ctx, request, "/control/filtering/set_rules", map[string]any{"rules": desired.Shared.Filtering.UserRules}); err != nil {
		return err
	}
	if desired.SchemaVersion >= configuration.SchemaVersion {
		if err := r.reconcileClients(ctx, request, desired.Shared.Clients); err != nil {
			return err
		}
		if err := r.reconcileRewrites(ctx, request, desired.Shared.RewritesEnabled, desired.Shared.Rewrites); err != nil {
			return err
		}
		if err := r.put(ctx, request, "/control/blocked_services/update", blockedServicesPayload(desired.Shared.Services)); err != nil {
			return err
		}
		if err := r.setEnabled(ctx, request, "/control/safebrowsing", desired.Shared.Services.SafeBrowsing); err != nil {
			return err
		}
		if err := r.setEnabled(ctx, request, "/control/parental", desired.Shared.Services.ParentalControl); err != nil {
			return err
		}
		if err := r.put(ctx, request, "/control/safesearch/settings", safeSearchPayload(desired.Shared.Services.SafeSearch)); err != nil {
			return err
		}
		if err := r.updatePolicy(ctx, request, "/control/querylog/config", "/control/querylog/config/update", desired.Shared.QueryLog.Enabled, desired.Shared.QueryLog.IntervalMillis, desired.Shared.QueryLog.Ignored, desired.Shared.QueryLog.IgnoredEnabled, map[string]any{"anonymize_client_ip": desired.Shared.QueryLog.AnonymizeClientIP}); err != nil {
			return err
		}
		if err := r.updatePolicy(ctx, request, "/control/stats/config", "/control/stats/config/update", desired.Shared.Statistics.Enabled, desired.Shared.Statistics.IntervalMillis, desired.Shared.Statistics.Ignored, desired.Shared.Statistics.IgnoredEnabled, nil); err != nil {
			return err
		}
	}
	dnsPayload := map[string]any{"upstream_dns": desired.Shared.DNS.UpstreamDNS, "bootstrap_dns": desired.Shared.DNS.BootstrapDNS, "fallback_dns": desired.Shared.DNS.FallbackDNS, "local_ptr_upstreams": desired.Shared.DNS.PrivateReverseDNS}
	if desired.SchemaVersion >= configuration.SchemaVersion {
		dns := desired.Shared.DNS
		for key, value := range map[string]any{"protection_enabled": dns.ProtectionEnabled, "ratelimit": dns.RateLimit, "ratelimit_subnet_len_ipv4": dns.RateLimitIPv4, "ratelimit_subnet_len_ipv6": dns.RateLimitIPv6, "ratelimit_whitelist": dns.RateLimitAllowlist, "blocking_mode": dns.BlockingMode, "blocking_ipv4": dns.BlockingIPv4, "blocking_ipv6": dns.BlockingIPv6, "blocked_response_ttl": dns.BlockedResponseTTL, "edns_cs_enabled": dns.EDNSClientSubnet, "edns_cs_use_custom": dns.EDNSUseCustom, "edns_cs_custom_ip": dns.EDNSCustomIP, "disable_ipv6": dns.DisableIPv6, "dnssec_enabled": dns.DNSSECEnabled, "cache_size": dns.CacheSize, "cache_ttl_min": dns.CacheTTLMin, "cache_ttl_max": dns.CacheTTLMax, "cache_optimistic": dns.CacheOptimistic, "upstream_mode": dns.UpstreamMode, "use_private_ptr_resolvers": dns.UsePrivateReverse, "resolve_clients": dns.ResolveClients} {
			dnsPayload[key] = value
		}
		if currentDNS.CacheEnabled != nil {
			dnsPayload["cache_enabled"] = dns.CacheEnabled
		}
		if currentDNS.UpstreamTimeout != nil {
			dnsPayload["upstream_timeout"] = dns.UpstreamTimeout
		}
	}
	if err := r.post(ctx, request, "/control/dns_config", dnsPayload); err != nil {
		return err
	}
	if desired.SchemaVersion >= configuration.SchemaVersion && desired.NodeSpecific.DHCP != nil {
		if err := r.reconcileDHCP(ctx, request, *desired.NodeSpecific.DHCP); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConfigurationReader) RefreshFilters(ctx context.Context, request domain.NodeProbeRequest, whitelist bool) error {
	return r.post(ctx, request, "/control/filtering/refresh", map[string]any{"whitelist": whitelist})
}

func (r *ConfigurationReader) TestUpstreamDNS(ctx context.Context, request domain.NodeProbeRequest, input operations.UpstreamInput) ([]operations.ResolverResult, error) {
	payload := map[string]any{
		"upstream_dns": input.UpstreamDNS, "bootstrap_dns": input.BootstrapDNS,
		"fallback_dns": input.FallbackDNS, "private_upstream": input.PrivateReverseDNS,
	}
	var response map[string]string
	if err := r.postOperationalResource(ctx, request, "/control/test_upstream_dns", payload, &response); err != nil {
		return nil, err
	}
	results := make([]operations.ResolverResult, 0, len(input.UpstreamDNS))
	for index, upstream := range input.UpstreamDNS {
		statuses, ok := operationalUpstreamStatuses(response, upstream)
		if !ok {
			return nil, domain.NewError(domain.ErrorNodeResponse, "the node upstream test response omitted a requested resolver")
		}
		if len(statuses) == 0 {
			continue
		}
		result := operations.ResolverResult{ResolverID: fmt.Sprintf("upstream-%d", index+1), Status: "succeeded"}
		for _, status := range statuses {
			if strings.TrimSpace(status) != "OK" {
				result.Status, result.ErrorCode = "failed", "UPSTREAM_TEST_FAILED"
				break
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func operationalUpstreamStatuses(response map[string]string, requested string) (statuses []string, ok bool) {
	candidates := operationalResolverCandidates(requested)
	if len(candidates) == 0 {
		return nil, true
	}
	statuses = make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		status, found := operationalUpstreamStatus(response, candidate)
		if !found {
			if len(candidates) == 1 && strings.HasPrefix(strings.ToLower(candidate), "sdns://") && len(response) > 0 {
				statuses = statuses[:0]
				for _, returnedStatus := range response {
					statuses = append(statuses, returnedStatus)
				}
				return statuses, true
			}
			return nil, false
		}
		statuses = append(statuses, status)
	}
	return statuses, true
}

func operationalResolverCandidates(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") {
		return nil
	}
	if strings.HasPrefix(value, "[/") {
		closingBracket := strings.Index(value, "]")
		if closingBracket >= 0 {
			resolvers := strings.TrimSpace(value[closingBracket+1:])
			if resolvers == "" || resolvers == "#" {
				return nil
			}
			return strings.Fields(resolvers)
		}
	}
	return []string{value}
}

func operationalUpstreamStatus(response map[string]string, requested string) (status string, ok bool) {
	if status, ok = response[requested]; ok {
		return status, true
	}
	canonicalRequested := canonicalOperationalUpstream(requested)
	for returned, returnedStatus := range response {
		if canonicalOperationalUpstream(returned) == canonicalRequested {
			return returnedStatus, true
		}
	}
	return "", false
}

func canonicalOperationalUpstream(value string) string {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "://") {
		return canonicalPlainOperationalUpstream(value)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme == "sdns" {
		return value
	}
	if parsed.Scheme == "h3" {
		parsed.Scheme = "https"
	}
	parsed.User = nil
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port == defaultUpstreamPort(parsed.Scheme) {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	parsed.Host = hostname
	if port != "" {
		parsed.Host += ":" + port
	}
	return parsed.String()
}

func canonicalPlainOperationalUpstream(value string) string {
	hostname, port, err := net.SplitHostPort(value)
	if err != nil {
		if address, parseErr := netip.ParseAddr(value); parseErr == nil {
			hostname = address.String()
		} else {
			hostname = value
		}
		port = ""
	}
	hostname = strings.ToLower(hostname)
	if port == "53" {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + strings.Trim(hostname, "[]") + "]"
	}
	if port != "" {
		hostname += ":" + port
	}
	return "udp://" + hostname
}

func defaultUpstreamPort(scheme string) string {
	switch scheme {
	case "https", "h3":
		return "443"
	case "tls", "quic":
		return "853"
	case "tcp", "udp":
		return "53"
	default:
		return ""
	}
}

func (r *ConfigurationReader) ClearDNSCache(ctx context.Context, request domain.NodeProbeRequest) error {
	return r.post(ctx, request, "/control/cache_clear", nil)
}

func (r *ConfigurationReader) ClearQueryLog(ctx context.Context, request domain.NodeProbeRequest) error {
	return r.post(ctx, request, "/control/querylog_clear", nil)
}

func (r *ConfigurationReader) ResetStatistics(ctx context.Context, request domain.NodeProbeRequest) error {
	return r.post(ctx, request, "/control/stats_reset", nil)
}

func (r *ConfigurationReader) TestHostFiltering(ctx context.Context, request domain.NodeProbeRequest, input operations.HostFilterInput) (operations.HostFilterResult, error) {
	query := url.Values{"name": []string{input.Hostname}}
	if input.Client != "" {
		query.Set("client", input.Client)
	}
	if input.QueryType != "" {
		query.Set("qtype", input.QueryType)
	}
	var response filterCheckHostResponse
	if err := r.getOperationalResource(ctx, request, "/control/filtering/check_host", query, &response); err != nil {
		return operations.HostFilterResult{}, err
	}
	if len(response.Rules) > 32 || len(response.IPAddresses) > 32 || len(response.Reason) > 128 || len(response.ServiceName) > 256 || len(response.CNAME) > 253 {
		return operations.HostFilterResult{}, domain.NewError(domain.ErrorNodeResponse, "the node host-filter response exceeded safe result limits")
	}
	if strings.ContainsAny(response.Reason, "\r\n\t") || strings.ContainsAny(response.ServiceName, "\r\n\t") || strings.ContainsAny(response.CNAME, "\r\n\t") {
		return operations.HostFilterResult{}, domain.NewError(domain.ErrorNodeResponse, "the node host-filter response contained unsafe text")
	}
	for _, address := range response.IPAddresses {
		if _, err := netip.ParseAddr(address); err != nil {
			return operations.HostFilterResult{}, domain.NewError(domain.ErrorNodeResponse, "the node host-filter response contained an invalid address")
		}
	}
	result := operations.HostFilterResult{Reason: response.Reason, ServiceName: response.ServiceName, CanonicalName: response.CNAME, Rules: []operations.MatchedRule{}, IPAddresses: append([]string(nil), response.IPAddresses...)}
	for _, rule := range response.Rules {
		if len(rule.Text) > 2048 || strings.ContainsAny(rule.Text, "\r\n") {
			return operations.HostFilterResult{}, domain.NewError(domain.ErrorNodeResponse, "the node host-filter response contained an unsafe rule")
		}
		result.Rules = append(result.Rules, operations.MatchedRule{Text: rule.Text, FilterListID: rule.FilterListID})
	}
	if len(result.Rules) == 0 && response.Rule != "" {
		if len(response.Rule) > 2048 || strings.ContainsAny(response.Rule, "\r\n") {
			return operations.HostFilterResult{}, domain.NewError(domain.ErrorNodeResponse, "the node host-filter response contained an unsafe rule")
		}
		result.Rules = append(result.Rules, operations.MatchedRule{Text: response.Rule, FilterListID: response.FilterID})
	}
	result.Matched = len(result.Rules) > 0 || result.ServiceName != "" || result.CanonicalName != "" || len(result.IPAddresses) > 0
	return result, nil
}

func (r *ConfigurationReader) getOperationalResource(ctx context.Context, request domain.NodeProbeRequest, path string, query url.Values, target any) error {
	transport, err := r.probe.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := configurationEndpoint(request.BaseURL, path)
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	parsed.RawQuery = query.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create AdGuard Home operational request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: transport, Timeout: r.probe.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return domain.NewError(domain.ErrorNodeAuth, "the node rejected its stored credentials")
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNotImplemented {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return nodeAPIError(domain.ErrorCapability, http.MethodGet, path, response.StatusCode, response.Header.Get("Content-Type"), "is not implemented by this node", nil)
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, path, response.StatusCode, response.Header.Get("Content-Type"), "returned an unexpected HTTP status", nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxConfigurationBody+1))
	if err != nil || len(body) > maxConfigurationBody {
		return domain.NewError(domain.ErrorNodeResponse, "the node operational response could not be read safely")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, path, response.StatusCode, response.Header.Get("Content-Type"), "returned invalid JSON", err)
	}
	return nil
}

func (r *ConfigurationReader) ResetDHCPLeases(ctx context.Context, request domain.NodeProbeRequest) error {
	return r.post(ctx, request, "/control/dhcp/reset_leases", nil)
}

func (r *ConfigurationReader) ResetDHCPConfiguration(ctx context.Context, request domain.NodeProbeRequest) error {
	return r.post(ctx, request, "/control/dhcp/reset", nil)
}

func (r *ConfigurationReader) reconcileFilterURLs(ctx context.Context, request domain.NodeProbeRequest, current filterStatusResponse, desired []string, whitelist bool) error {
	targets := make(map[string]struct{}, len(desired))
	currentItems := current.Filters
	for _, item := range current.WhitelistFilters {
		item.Whitelist = true
		currentItems = append(currentItems, item)
	}
	for _, target := range desired {
		targets[strings.ToLower(target)] = struct{}{}
		found := false
		for _, item := range currentItems {
			if item.Whitelist == whitelist && strings.EqualFold(item.URL, target) {
				found = true
				if !item.Enabled {
					if err := r.post(ctx, request, "/control/filtering/set_url", map[string]any{"url": item.URL, "whitelist": whitelist, "data": map[string]any{"name": item.Name, "url": item.URL, "enabled": true}}); err != nil {
						return err
					}
				}
				break
			}
		}
		if !found {
			if err := r.post(ctx, request, "/control/filtering/add_url", map[string]any{"name": "Managed by Atlas DNS Controller", "url": target, "whitelist": whitelist}); err != nil {
				return err
			}
		}
	}
	for _, item := range currentItems {
		_, wanted := targets[strings.ToLower(item.URL)]
		if item.Whitelist == whitelist && item.Enabled && !wanted {
			if err := r.post(ctx, request, "/control/filtering/set_url", map[string]any{"url": item.URL, "whitelist": whitelist, "data": map[string]any{"name": item.Name, "url": item.URL, "enabled": false}}); err != nil {
				return err
			}
		}
	}
	return nil
}

func clientPayload(client configuration.PersistentClient) map[string]any {
	return map[string]any{"name": client.Name, "ids": client.IDs, "use_global_settings": client.UseGlobalSettings, "filtering_enabled": client.FilteringEnabled, "parental_enabled": client.ParentalEnabled, "safebrowsing_enabled": client.SafeBrowsingEnabled, "safe_search": safeSearchPayload(client.SafeSearch), "use_global_blocked_services": client.UseGlobalBlockedServices, "blocked_services": client.BlockedServices, "blocked_services_schedule": schedulePayload(client.BlockedServicesSchedule), "upstreams": client.Upstreams, "upstreams_cache_enabled": client.UpstreamsCacheEnabled, "upstreams_cache_size": client.UpstreamsCacheSize, "tags": client.Tags, "ignore_querylog": client.IgnoreQueryLog, "ignore_statistics": client.IgnoreStatistics}
}

func (r *ConfigurationReader) reconcileClients(ctx context.Context, request domain.NodeProbeRequest, desired []configuration.PersistentClient) error {
	var current clientsResponse
	if err := r.get(ctx, request, "/control/clients", &current); err != nil {
		return err
	}
	existing := map[string]clientResponse{}
	for _, item := range current.Clients {
		existing[strings.ToLower(item.Name)] = item
	}
	targets := map[string]bool{}
	for _, item := range desired {
		key := strings.ToLower(item.Name)
		targets[key] = true
		if old, ok := existing[key]; ok {
			if err := r.post(ctx, request, "/control/clients/update", map[string]any{"name": old.Name, "data": clientPayload(item)}); err != nil {
				return err
			}
		} else if err := r.post(ctx, request, "/control/clients/add", clientPayload(item)); err != nil {
			return err
		}
	}
	for _, item := range current.Clients {
		if !targets[strings.ToLower(item.Name)] {
			if err := r.post(ctx, request, "/control/clients/delete", map[string]any{"name": item.Name}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ConfigurationReader) reconcileRewrites(ctx context.Context, request domain.NodeProbeRequest, enabled bool, desired []configuration.Rewrite) error {
	var current []rewriteResponse
	if err := r.get(ctx, request, "/control/rewrite/list", &current); err != nil {
		return err
	}
	settingsSupported, err := r.getOptionalEndpoint(ctx, request, "/control/rewrite/settings", &rewriteSettingsResponse{})
	if err != nil {
		return err
	}
	if settingsSupported {
		if err := r.put(ctx, request, "/control/rewrite/settings/update", map[string]any{"enabled": enabled}); err != nil {
			return err
		}
	}
	key := func(domain, answer string) string { return strings.ToLower(domain) + "\x00" + strings.ToLower(answer) }
	existing, targets := map[string]rewriteResponse{}, map[string]bool{}
	for _, item := range current {
		existing[key(item.Domain, item.Answer)] = item
	}
	for _, item := range desired {
		k := key(item.Domain, item.Answer)
		targets[k] = true
		old, ok := existing[k]
		payload := map[string]any{"domain": item.Domain, "answer": item.Answer}
		if settingsSupported {
			payload["enabled"] = item.Enabled
		}
		if !ok {
			if err := r.post(ctx, request, "/control/rewrite/add", payload); err != nil {
				return err
			}
		} else if settingsSupported && valueOrDefault(old.Enabled, true) != item.Enabled {
			if err := r.put(ctx, request, "/control/rewrite/update", map[string]any{"target": map[string]any{"domain": old.Domain, "answer": old.Answer}, "update": payload}); err != nil {
				return err
			}
		}
	}
	for _, item := range current {
		if !targets[key(item.Domain, item.Answer)] {
			if err := r.post(ctx, request, "/control/rewrite/delete", map[string]any{"domain": item.Domain, "answer": item.Answer}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ConfigurationReader) updatePolicy(ctx context.Context, request domain.NodeProbeRequest, readPath, updatePath string, enabled bool, interval int64, ignored []string, ignoredEnabled bool, extra map[string]any) error {
	var current policyResponse
	if err := r.get(ctx, request, readPath, &current); err != nil {
		return err
	}
	payload := map[string]any{"enabled": enabled, "interval": interval, "ignored": ignored}
	for key, value := range extra {
		payload[key] = value
	}
	if current.IgnoredEnabled != nil {
		payload["ignored_enabled"] = ignoredEnabled
	}
	return r.put(ctx, request, updatePath, payload)
}

func safeSearchPayload(value configuration.SafeSearch) map[string]any {
	return map[string]any{"enabled": value.Enabled, "bing": value.Bing, "duckduckgo": value.DuckDuckGo, "ecosia": value.Ecosia, "google": value.Google, "pixabay": value.Pixabay, "yandex": value.Yandex, "youtube": value.YouTube}
}

func blockedServicesPayload(value configuration.Services) map[string]any {
	return map[string]any{"ids": value.BlockedServiceIDs, "schedule": schedulePayload(value.BlockedSchedule)}
}

func schedulePayload(value configuration.Schedule) map[string]any {
	result := map[string]any{"time_zone": value.TimeZone}
	for day, period := range value.Days {
		result[day] = map[string]any{"start": period.Start, "end": period.End}
	}
	return result
}

func (r *ConfigurationReader) setEnabled(ctx context.Context, request domain.NodeProbeRequest, prefix string, enabled bool) error {
	suffix := "/disable"
	if enabled {
		suffix = "/enable"
	}
	return r.send(ctx, request, http.MethodPost, prefix+suffix, nil)
}

func (r *ConfigurationReader) reconcileDHCP(ctx context.Context, request domain.NodeProbeRequest, desired configuration.DHCPConfig) error {
	var current dhcpStatusResponse
	if err := r.get(ctx, request, "/control/dhcp/status", &current); err != nil {
		return err
	}
	if !dhcpConfigurationMatches(current, desired) {
		payload := map[string]any{"enabled": desired.Enabled, "interface_name": desired.InterfaceName, "v4": map[string]any{"gateway_ip": desired.IPv4.Gateway, "subnet_mask": desired.IPv4.SubnetMask, "range_start": desired.IPv4.RangeStart, "range_end": desired.IPv4.RangeEnd, "lease_duration": desired.IPv4.LeaseDuration}, "v6": map[string]any{"range_start": desired.IPv6.RangeStart, "lease_duration": desired.IPv6.LeaseDuration}}
		if err := r.post(ctx, request, "/control/dhcp/set_config", payload); err != nil {
			return err
		}
	}
	byMAC, targets := map[string]dhcpLeaseResponse{}, map[string]bool{}
	for _, lease := range current.StaticLeases {
		byMAC[strings.ToLower(lease.MAC)] = lease
	}
	for _, lease := range desired.StaticLeases {
		key := strings.ToLower(lease.MAC)
		targets[key] = true
		if old, ok := byMAC[key]; ok && (old.IP != lease.IP || old.Hostname != lease.Hostname) {
			if err := r.post(ctx, request, "/control/dhcp/remove_static_lease", map[string]any{"mac": old.MAC, "ip": old.IP, "hostname": old.Hostname}); err != nil {
				return err
			}
			delete(byMAC, key)
		}
		if _, ok := byMAC[key]; !ok {
			if err := r.post(ctx, request, "/control/dhcp/add_static_lease", map[string]any{"mac": lease.MAC, "ip": lease.IP, "hostname": lease.Hostname}); err != nil {
				return err
			}
		}
	}
	for key, lease := range byMAC {
		if !targets[key] {
			if err := r.post(ctx, request, "/control/dhcp/remove_static_lease", map[string]any{"mac": lease.MAC, "ip": lease.IP, "hostname": lease.Hostname}); err != nil {
				return err
			}
		}
	}
	return nil
}

func dhcpConfigurationMatches(current dhcpStatusResponse, desired configuration.DHCPConfig) bool {
	return current.Enabled == desired.Enabled &&
		current.InterfaceName == desired.InterfaceName &&
		current.V4.Gateway == desired.IPv4.Gateway &&
		current.V4.SubnetMask == desired.IPv4.SubnetMask &&
		current.V4.RangeStart == desired.IPv4.RangeStart &&
		current.V4.RangeEnd == desired.IPv4.RangeEnd &&
		current.V4.LeaseDuration == desired.IPv4.LeaseDuration &&
		current.V6.RangeStart == desired.IPv6.RangeStart &&
		current.V6.LeaseDuration == desired.IPv6.LeaseDuration
}

func (r *ConfigurationReader) post(ctx context.Context, request domain.NodeProbeRequest, path string, payload any) error {
	return r.send(ctx, request, http.MethodPost, path, payload)
}

func (r *ConfigurationReader) put(ctx context.Context, request domain.NodeProbeRequest, path string, payload any) error {
	return r.send(ctx, request, http.MethodPut, path, payload)
}

func (r *ConfigurationReader) postResource(ctx context.Context, request domain.NodeProbeRequest, path string, payload, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode AdGuard Home read-only request: %w", err)
	}
	transport, err := r.probe.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := configurationEndpoint(request.BaseURL, path)
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create AdGuard Home read-only request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: transport, Timeout: r.probe.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return domain.NewError(domain.ErrorNodeAuth, "the node rejected its stored credentials")
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNotImplemented {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return domain.NewError(domain.ErrorCapability, "active DHCP detection is not supported by this node")
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return domain.NewError(domain.ErrorNodeResponse, "the node active DHCP endpoint returned an unexpected status")
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxConfigurationBody+1))
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node active DHCP response could not be read")
	}
	if len(responseBody) > maxConfigurationBody {
		return domain.NewError(domain.ErrorNodeResponse, "the node active DHCP response was too large")
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node returned invalid active DHCP JSON")
	}
	return nil
}

func (r *ConfigurationReader) postOperationalResource(ctx context.Context, request domain.NodeProbeRequest, path string, payload, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode AdGuard Home operational request: %w", err)
	}
	transport, err := r.probe.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := configurationEndpoint(request.BaseURL, path)
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create AdGuard Home operational request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: transport, Timeout: r.probe.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return domain.NewError(domain.ErrorNodeAuth, "the node rejected its stored credentials")
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNotImplemented {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return domain.NewError(domain.ErrorCapability, "the node does not support this operational command")
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return domain.NewError(domain.ErrorNodeResponse, "the node operational endpoint returned an unexpected status")
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxConfigurationBody+1))
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node operational response could not be read")
	}
	if len(responseBody) > maxConfigurationBody {
		return domain.NewError(domain.ErrorNodeResponse, "the node operational response was too large")
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node returned invalid operational JSON")
	}
	return nil
}

func (r *ConfigurationReader) send(ctx context.Context, request domain.NodeProbeRequest, method, path string, payload any) error {
	var reader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode AdGuard Home configuration request: %w", err)
		}
		reader = bytes.NewReader(body)
	}
	transport, err := r.probe.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := configurationEndpoint(request.BaseURL, path)
	if err != nil {
		return domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("create AdGuard Home mutation request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	if payload != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Transport: transport, Timeout: r.probe.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return domain.NewError(domain.ErrorNodeAuth, "the node rejected its stored credentials")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.NewError(domain.ErrorNodeApply, fmt.Sprintf("AdGuard Home rejected %s %s with HTTP %d", method, path, response.StatusCode))
	}
	return nil
}

func (r *ConfigurationReader) get(ctx context.Context, request domain.NodeProbeRequest, path string, target any) error {
	_, err := r.getResource(ctx, request, path, target, nil)
	return err
}

func (r *ConfigurationReader) getOptional(ctx context.Context, request domain.NodeProbeRequest, path string, target any) (bool, error) {
	return r.getResource(ctx, request, path, target, map[int]bool{http.StatusNotFound: true, http.StatusInternalServerError: true, http.StatusNotImplemented: true})
}

func (r *ConfigurationReader) getOptionalEndpoint(ctx context.Context, request domain.NodeProbeRequest, path string, target any) (bool, error) {
	return r.getResource(ctx, request, path, target, map[int]bool{http.StatusNotFound: true, http.StatusNotImplemented: true})
}

func (r *ConfigurationReader) getResource(ctx context.Context, request domain.NodeProbeRequest, path string, target any, unavailable map[int]bool) (bool, error) {
	transport, err := r.probe.transport(request.CertificatePolicy, request.CustomCAPEM)
	if err != nil {
		return false, err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := configurationEndpoint(request.BaseURL, path)
	if err != nil {
		return false, domain.NewError(domain.ErrorNodeResponse, "the node URL is invalid")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("create AdGuard Home configuration request: %w", err)
	}
	httpRequest.SetBasicAuth(request.Credentials.Username, request.Credentials.Password)
	httpRequest.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: transport, Timeout: r.probe.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		return false, classifyNetworkError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return false, domain.NewError(domain.ErrorNodeAuth, "the node rejected its stored credentials")
	}
	if unavailable[response.StatusCode] {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConfigurationBody))
		return false, nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, path, response.StatusCode, response.Header.Get("Content-Type"), "returned an unexpected HTTP status", nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxConfigurationBody+1))
	if err != nil {
		return false, domain.NewError(domain.ErrorNodeResponse, "the node configuration response could not be read")
	}
	if len(body) > maxConfigurationBody {
		return false, domain.NewError(domain.ErrorNodeResponse, "the node configuration response was too large")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return false, nodeAPIError(domain.ErrorNodeResponse, http.MethodGet, path, response.StatusCode, response.Header.Get("Content-Type"), "returned invalid JSON", err)
	}
	return true, nil
}

func configurationEndpoint(baseURL, path string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String(), nil
}
