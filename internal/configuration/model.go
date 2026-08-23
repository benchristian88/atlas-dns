package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	LegacySchemaVersion = 1
	SchemaVersion       = 2
)

type Document struct {
	SchemaVersion int           `json:"schemaVersion"`
	Shared        Shared        `json:"shared"`
	NodeSpecific  NodeSpecific  `json:"nodeSpecific"`
	ObservedOnly  ObservedOnly  `json:"observedOnly"`
	Unsupported   []Unsupported `json:"unsupported"`
}

// DesiredDocument is authoritative cluster intent.  Observed Document remains
// the frozen Release 0.2 per-node shape; keeping the two types distinct stops
// node-generated and observed-only values from becoming desired state.
type DesiredDocument struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Shared        Shared                  `json:"shared"`
	NodeOverrides map[string]NodeSpecific `json:"nodeOverrides"`
	Unsupported   []Unsupported           `json:"unsupported"`
}

type Shared struct {
	DNS             DNS                `json:"dns"`
	Filtering       Filtering          `json:"filtering"`
	Clients         []PersistentClient `json:"clients"`
	RewritesEnabled bool               `json:"rewritesEnabled"`
	Rewrites        []Rewrite          `json:"rewrites"`
	Services        Services           `json:"services"`
	QueryLog        QueryLogPolicy     `json:"queryLog"`
	Statistics      StatisticsPolicy   `json:"statistics"`
}

type DNS struct {
	UpstreamDNS        []string `json:"upstreamDns"`
	BootstrapDNS       []string `json:"bootstrapDns"`
	FallbackDNS        []string `json:"fallbackDns"`
	PrivateReverseDNS  []string `json:"privateReverseDns"`
	ProtectionEnabled  bool     `json:"protectionEnabled,omitempty"`
	RateLimit          int      `json:"rateLimit,omitempty"`
	RateLimitIPv4      int      `json:"rateLimitSubnetLengthIpv4,omitempty"`
	RateLimitIPv6      int      `json:"rateLimitSubnetLengthIpv6,omitempty"`
	RateLimitAllowlist []string `json:"rateLimitAllowlist,omitempty"`
	BlockingMode       string   `json:"blockingMode,omitempty"`
	BlockingIPv4       string   `json:"blockingIpv4,omitempty"`
	BlockingIPv6       string   `json:"blockingIpv6,omitempty"`
	BlockedResponseTTL int      `json:"blockedResponseTtl,omitempty"`
	EDNSClientSubnet   bool     `json:"ednsClientSubnet,omitempty"`
	EDNSUseCustom      bool     `json:"ednsUseCustom,omitempty"`
	EDNSCustomIP       string   `json:"ednsCustomIp,omitempty"`
	DisableIPv6        bool     `json:"disableIpv6,omitempty"`
	DNSSECEnabled      bool     `json:"dnssecEnabled,omitempty"`
	CacheSize          int      `json:"cacheSize,omitempty"`
	CacheEnabled       bool     `json:"cacheEnabled"`
	CacheTTLMin        int      `json:"cacheTtlMin,omitempty"`
	CacheTTLMax        int      `json:"cacheTtlMax,omitempty"`
	CacheOptimistic    bool     `json:"cacheOptimistic,omitempty"`
	UpstreamMode       string   `json:"upstreamMode,omitempty"`
	UsePrivateReverse  bool     `json:"usePrivateReverseResolvers,omitempty"`
	ResolveClients     bool     `json:"resolveClients,omitempty"`
	UpstreamTimeout    int      `json:"upstreamTimeoutSeconds,omitempty"`
}

type Filtering struct {
	Enabled        bool     `json:"enabled"`
	UpdateInterval int      `json:"updateIntervalHours"`
	FilterURLs     []string `json:"filterUrls"`
	WhitelistURLs  []string `json:"whitelistUrls"`
	UserRules      []string `json:"userRules"`
}

type SafeSearch struct {
	Enabled    bool `json:"enabled"`
	Bing       bool `json:"bing"`
	DuckDuckGo bool `json:"duckDuckGo"`
	Ecosia     bool `json:"ecosia,omitempty"`
	Google     bool `json:"google"`
	Pixabay    bool `json:"pixabay"`
	Yandex     bool `json:"yandex"`
	YouTube    bool `json:"youtube"`
}

type PersistentClient struct {
	Name                     string     `json:"name"`
	IDs                      []string   `json:"ids"`
	UseGlobalSettings        bool       `json:"useGlobalSettings"`
	FilteringEnabled         bool       `json:"filteringEnabled"`
	ParentalEnabled          bool       `json:"parentalEnabled"`
	SafeBrowsingEnabled      bool       `json:"safeBrowsingEnabled"`
	SafeSearch               SafeSearch `json:"safeSearch"`
	UseGlobalBlockedServices bool       `json:"useGlobalBlockedServices"`
	BlockedServices          []string   `json:"blockedServices"`
	BlockedServicesSchedule  Schedule   `json:"blockedServicesSchedule"`
	Upstreams                []string   `json:"upstreams"`
	UpstreamsCacheEnabled    bool       `json:"upstreamsCacheEnabled"`
	UpstreamsCacheSize       int        `json:"upstreamsCacheSize"`
	Tags                     []string   `json:"tags"`
	IgnoreQueryLog           bool       `json:"ignoreQueryLog"`
	IgnoreStatistics         bool       `json:"ignoreStatistics"`
}

type Rewrite struct {
	Domain  string `json:"domain"`
	Answer  string `json:"answer"`
	Enabled bool   `json:"enabled"`
}

type DayRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type Schedule struct {
	TimeZone string              `json:"timeZone"`
	Days     map[string]DayRange `json:"days"`
}

type Services struct {
	BlockedServiceIDs []string   `json:"blockedServiceIds"`
	BlockedSchedule   Schedule   `json:"blockedSchedule"`
	SafeBrowsing      bool       `json:"safeBrowsing"`
	ParentalControl   bool       `json:"parentalControl"`
	SafeSearch        SafeSearch `json:"safeSearch"`
}

type QueryLogPolicy struct {
	Enabled           bool     `json:"enabled"`
	IntervalMillis    int64    `json:"intervalMillis"`
	AnonymizeClientIP bool     `json:"anonymizeClientIp"`
	Ignored           []string `json:"ignored"`
	IgnoredEnabled    bool     `json:"ignoredEnabled"`
}

type StatisticsPolicy struct {
	Enabled        bool     `json:"enabled"`
	IntervalMillis int64    `json:"intervalMillis"`
	Ignored        []string `json:"ignored"`
	IgnoredEnabled bool     `json:"ignoredEnabled"`
}

type NodeSpecific struct {
	BindHosts []string    `json:"bindHosts"`
	DNSPort   int         `json:"dnsPort"`
	DHCP      *DHCPConfig `json:"dhcp,omitempty"`
}

type ObservedOnly struct {
	ProductVersion          string      `json:"productVersion"`
	ProtectionDisabledUntil string      `json:"protectionDisabledUntil,omitempty"`
	TLS                     TLSStatus   `json:"tls"`
	DHCPLeases              []DHCPLease `json:"dhcpLeases,omitempty"`
}

type DHCPConfig struct {
	Enabled       bool              `json:"enabled"`
	InterfaceName string            `json:"interfaceName"`
	IPv4          DHCPIPv4          `json:"ipv4"`
	IPv6          DHCPIPv6          `json:"ipv6"`
	StaticLeases  []DHCPStaticLease `json:"staticLeases"`
}

type DHCPIPv4 struct {
	Gateway       string `json:"gateway"`
	SubnetMask    string `json:"subnetMask"`
	RangeStart    string `json:"rangeStart"`
	RangeEnd      string `json:"rangeEnd"`
	LeaseDuration int64  `json:"leaseDurationSeconds"`
}

type DHCPIPv6 struct {
	RangeStart    string `json:"rangeStart"`
	LeaseDuration int64  `json:"leaseDurationSeconds"`
}

type DHCPStaticLease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

type DHCPLease struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	ExpiresAt string `json:"expiresAt"`
}

type TLSStatus struct {
	Enabled          bool     `json:"enabled"`
	ServerName       string   `json:"serverName"`
	ForceHTTPS       bool     `json:"forceHttps"`
	HTTPSPort        int      `json:"httpsPort"`
	DNSOverTLSPort   int      `json:"dnsOverTlsPort"`
	DNSOverQUICPort  int      `json:"dnsOverQuicPort"`
	ServePlainDNS    bool     `json:"servePlainDns"`
	ValidCertificate bool     `json:"validCertificate"`
	ValidChain       bool     `json:"validChain"`
	ValidKey         bool     `json:"validKey"`
	ValidPair        bool     `json:"validPair"`
	Subject          string   `json:"subject,omitempty"`
	Issuer           string   `json:"issuer,omitempty"`
	NotBefore        string   `json:"notBefore,omitempty"`
	NotAfter         string   `json:"notAfter,omitempty"`
	DNSNames         []string `json:"dnsNames,omitempty"`
	Warning          string   `json:"warning,omitempty"`
}

type legacyDNS struct {
	UpstreamDNS       []string `json:"upstreamDns"`
	BootstrapDNS      []string `json:"bootstrapDns"`
	FallbackDNS       []string `json:"fallbackDns"`
	PrivateReverseDNS []string `json:"privateReverseDns"`
}

type legacyFiltering struct {
	Enabled        bool     `json:"enabled"`
	UpdateInterval int      `json:"updateIntervalHours"`
	FilterURLs     []string `json:"filterUrls"`
	UserRules      []string `json:"userRules"`
}

type legacyShared struct {
	DNS       legacyDNS       `json:"dns"`
	Filtering legacyFiltering `json:"filtering"`
}

func legacySharedFrom(value Shared) legacyShared {
	return legacyShared{DNS: legacyDNS{UpstreamDNS: value.DNS.UpstreamDNS, BootstrapDNS: value.DNS.BootstrapDNS, FallbackDNS: value.DNS.FallbackDNS, PrivateReverseDNS: value.DNS.PrivateReverseDNS}, Filtering: legacyFiltering{Enabled: value.Filtering.Enabled, UpdateInterval: value.Filtering.UpdateInterval, FilterURLs: value.Filtering.FilterURLs, UserRules: value.Filtering.UserRules}}
}

func (document Document) MarshalJSON() ([]byte, error) {
	if document.SchemaVersion != LegacySchemaVersion {
		type alias Document
		return json.Marshal(alias(document))
	}
	return json.Marshal(struct {
		SchemaVersion int          `json:"schemaVersion"`
		Shared        legacyShared `json:"shared"`
		NodeSpecific  NodeSpecific `json:"nodeSpecific"`
		ObservedOnly  struct {
			ProductVersion string `json:"productVersion"`
		} `json:"observedOnly"`
		Unsupported []Unsupported `json:"unsupported"`
	}{document.SchemaVersion, legacySharedFrom(document.Shared), NodeSpecific{BindHosts: document.NodeSpecific.BindHosts, DNSPort: document.NodeSpecific.DNSPort}, struct {
		ProductVersion string `json:"productVersion"`
	}{document.ObservedOnly.ProductVersion}, document.Unsupported})
}

func (document DesiredDocument) MarshalJSON() ([]byte, error) {
	if document.SchemaVersion != LegacySchemaVersion {
		type alias DesiredDocument
		return json.Marshal(alias(document))
	}
	overrides := make(map[string]NodeSpecific, len(document.NodeOverrides))
	for id, value := range document.NodeOverrides {
		overrides[id] = NodeSpecific{BindHosts: value.BindHosts, DNSPort: value.DNSPort}
	}
	return json.Marshal(struct {
		SchemaVersion int                     `json:"schemaVersion"`
		Shared        legacyShared            `json:"shared"`
		NodeOverrides map[string]NodeSpecific `json:"nodeOverrides"`
		Unsupported   []Unsupported           `json:"unsupported"`
	}{document.SchemaVersion, legacySharedFrom(document.Shared), overrides, document.Unsupported})
}

type Unsupported struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

type Ownership string

const (
	SharedManaged    Ownership = "shared_managed"
	NodeManaged      Ownership = "node_specific_managed"
	Observed         Ownership = "observed_only"
	UnsupportedField Ownership = "unsupported"
)

type Difference struct {
	Section string    `json:"section"`
	Field   string    `json:"field"`
	Scope   Ownership `json:"scope"`
	Left    any       `json:"left"`
	Right   any       `json:"right"`
	Summary string    `json:"summary"`
}

func Canonicalise(document Document) Document {
	if document.SchemaVersion == 0 {
		document.SchemaVersion = SchemaVersion
	}
	document.Shared.DNS.UpstreamDNS = cleanOrdered(document.Shared.DNS.UpstreamDNS)
	document.Shared.DNS.BootstrapDNS = cleanSet(document.Shared.DNS.BootstrapDNS)
	document.Shared.DNS.FallbackDNS = cleanOrdered(document.Shared.DNS.FallbackDNS)
	document.Shared.DNS.PrivateReverseDNS = cleanSet(document.Shared.DNS.PrivateReverseDNS)
	document.Shared.DNS.RateLimitAllowlist = cleanSet(document.Shared.DNS.RateLimitAllowlist)
	document.Shared.DNS.BlockingMode = strings.TrimSpace(document.Shared.DNS.BlockingMode)
	document.Shared.DNS.EDNSCustomIP = strings.TrimSpace(document.Shared.DNS.EDNSCustomIP)
	document.Shared.DNS.UpstreamMode = strings.TrimSpace(document.Shared.DNS.UpstreamMode)
	document.Shared.Filtering.FilterURLs = cleanSet(document.Shared.Filtering.FilterURLs)
	document.Shared.Filtering.WhitelistURLs = cleanSet(document.Shared.Filtering.WhitelistURLs)
	document.Shared.Filtering.UserRules = cleanOrdered(document.Shared.Filtering.UserRules)
	for index := range document.Shared.Clients {
		client := &document.Shared.Clients[index]
		client.Name = strings.TrimSpace(client.Name)
		client.IDs = cleanSet(client.IDs)
		client.BlockedServices = cleanSet(client.BlockedServices)
		client.BlockedServicesSchedule = canonicalSchedule(client.BlockedServicesSchedule)
		client.Upstreams = cleanOrdered(client.Upstreams)
		client.Tags = cleanSet(client.Tags)
	}
	sort.Slice(document.Shared.Clients, func(i, j int) bool {
		return strings.ToLower(document.Shared.Clients[i].Name) < strings.ToLower(document.Shared.Clients[j].Name)
	})
	for index := range document.Shared.Rewrites {
		document.Shared.Rewrites[index].Domain = strings.TrimSpace(document.Shared.Rewrites[index].Domain)
		document.Shared.Rewrites[index].Answer = strings.TrimSpace(document.Shared.Rewrites[index].Answer)
	}
	sort.Slice(document.Shared.Rewrites, func(i, j int) bool {
		if strings.EqualFold(document.Shared.Rewrites[i].Domain, document.Shared.Rewrites[j].Domain) {
			return strings.ToLower(document.Shared.Rewrites[i].Answer) < strings.ToLower(document.Shared.Rewrites[j].Answer)
		}
		return strings.ToLower(document.Shared.Rewrites[i].Domain) < strings.ToLower(document.Shared.Rewrites[j].Domain)
	})
	document.Shared.Services.BlockedServiceIDs = cleanSet(document.Shared.Services.BlockedServiceIDs)
	document.Shared.Services.BlockedSchedule = canonicalSchedule(document.Shared.Services.BlockedSchedule)
	document.Shared.QueryLog.Ignored = cleanSet(document.Shared.QueryLog.Ignored)
	document.Shared.Statistics.Ignored = cleanSet(document.Shared.Statistics.Ignored)
	document.NodeSpecific.BindHosts = cleanSet(document.NodeSpecific.BindHosts)
	if document.NodeSpecific.DHCP != nil {
		canonicaliseDHCP(document.NodeSpecific.DHCP)
	}
	document.ObservedOnly.TLS.DNSNames = cleanSet(document.ObservedOnly.TLS.DNSNames)
	if document.ObservedOnly.DHCPLeases == nil {
		document.ObservedOnly.DHCPLeases = []DHCPLease{}
	}
	sort.Slice(document.Unsupported, func(i, j int) bool {
		if document.Unsupported[i].Section == document.Unsupported[j].Section {
			return document.Unsupported[i].Reason < document.Unsupported[j].Reason
		}
		return document.Unsupported[i].Section < document.Unsupported[j].Section
	})
	return document
}

func CanonicaliseDesired(document DesiredDocument) DesiredDocument {
	if document.SchemaVersion == 0 {
		document.SchemaVersion = SchemaVersion
	}
	observed := Canonicalise(Document{SchemaVersion: document.SchemaVersion, Shared: document.Shared, Unsupported: document.Unsupported})
	document.Shared, document.Unsupported = observed.Shared, observed.Unsupported
	if document.NodeOverrides == nil {
		document.NodeOverrides = map[string]NodeSpecific{}
	}
	for nodeID, override := range document.NodeOverrides {
		override.BindHosts = cleanSet(override.BindHosts)
		if override.DHCP != nil {
			canonicaliseDHCP(override.DHCP)
		}
		document.NodeOverrides[nodeID] = override
	}
	return document
}

func DesiredFromObservation(nodeID string, document Document) DesiredDocument {
	desired := DesiredDocument{
		SchemaVersion: document.SchemaVersion,
		Shared:        document.Shared,
		NodeOverrides: map[string]NodeSpecific{nodeID: document.NodeSpecific},
		Unsupported:   document.Unsupported,
	}
	return CanonicaliseDesired(desired)
}

// ProjectDocument narrows a current observation to the feature boundary of a
// historical revision.  This lets schema-v1 active revisions continue to
// reconcile and roll back without treating newly observed v2 fields as drift.
func ProjectDocument(document Document, schemaVersion int) Document {
	document = Canonicalise(document)
	if schemaVersion == LegacySchemaVersion {
		return Canonicalise(Document{
			SchemaVersion: LegacySchemaVersion,
			Shared: Shared{
				DNS: DNS{
					UpstreamDNS: document.Shared.DNS.UpstreamDNS, BootstrapDNS: document.Shared.DNS.BootstrapDNS,
					FallbackDNS: document.Shared.DNS.FallbackDNS, PrivateReverseDNS: document.Shared.DNS.PrivateReverseDNS,
				},
				Filtering: Filtering{
					Enabled: document.Shared.Filtering.Enabled, UpdateInterval: document.Shared.Filtering.UpdateInterval,
					FilterURLs: document.Shared.Filtering.FilterURLs, UserRules: document.Shared.Filtering.UserRules,
				},
			},
			NodeSpecific: NodeSpecific{BindHosts: document.NodeSpecific.BindHosts, DNSPort: document.NodeSpecific.DNSPort},
			Unsupported:  document.Unsupported,
		})
	}
	document.SchemaVersion = SchemaVersion
	return document
}

func Effective(document DesiredDocument, nodeID string) (Document, error) {
	document = CanonicaliseDesired(document)
	override, ok := document.NodeOverrides[nodeID]
	if !ok {
		return Document{}, fmt.Errorf("node %s has no desired override", nodeID)
	}
	return Canonicalise(Document{
		SchemaVersion: document.SchemaVersion,
		Shared:        document.Shared,
		NodeSpecific:  override,
		Unsupported:   document.Unsupported,
	}), nil
}

func Marshal(document Document) ([]byte, string, error) {
	body, err := json.Marshal(Canonicalise(document))
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func MarshalDesired(document DesiredDocument) ([]byte, string, error) {
	body, err := json.Marshal(CanonicaliseDesired(document))
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func Diff(left, right Document) []Difference {
	left, right = Canonicalise(left), Canonicalise(right)
	diffs := make([]Difference, 0)
	add := func(section, field string, scope Ownership, a, b any) {
		ab, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		if string(ab) != string(bb) {
			diffs = append(diffs, Difference{Section: section, Field: field, Scope: scope, Left: a, Right: b, Summary: field + " differs"})
		}
	}
	add("DNS", "Upstream DNS", SharedManaged, left.Shared.DNS.UpstreamDNS, right.Shared.DNS.UpstreamDNS)
	add("DNS", "Bootstrap DNS", SharedManaged, left.Shared.DNS.BootstrapDNS, right.Shared.DNS.BootstrapDNS)
	add("DNS", "Fallback DNS", SharedManaged, left.Shared.DNS.FallbackDNS, right.Shared.DNS.FallbackDNS)
	add("DNS", "Private reverse DNS", SharedManaged, left.Shared.DNS.PrivateReverseDNS, right.Shared.DNS.PrivateReverseDNS)
	if left.SchemaVersion >= SchemaVersion || right.SchemaVersion >= SchemaVersion {
		add("DNS", "Protection enabled", SharedManaged, left.Shared.DNS.ProtectionEnabled, right.Shared.DNS.ProtectionEnabled)
		add("DNS", "Rate limit", SharedManaged, left.Shared.DNS.RateLimit, right.Shared.DNS.RateLimit)
		add("DNS", "Rate-limit IPv4 subnet", SharedManaged, left.Shared.DNS.RateLimitIPv4, right.Shared.DNS.RateLimitIPv4)
		add("DNS", "Rate-limit IPv6 subnet", SharedManaged, left.Shared.DNS.RateLimitIPv6, right.Shared.DNS.RateLimitIPv6)
		add("DNS", "Rate-limit allowlist", SharedManaged, left.Shared.DNS.RateLimitAllowlist, right.Shared.DNS.RateLimitAllowlist)
		add("DNS", "Blocking mode", SharedManaged, left.Shared.DNS.BlockingMode, right.Shared.DNS.BlockingMode)
		add("DNS", "Blocking IPv4", SharedManaged, left.Shared.DNS.BlockingIPv4, right.Shared.DNS.BlockingIPv4)
		add("DNS", "Blocking IPv6", SharedManaged, left.Shared.DNS.BlockingIPv6, right.Shared.DNS.BlockingIPv6)
		add("DNS", "Blocked response TTL", SharedManaged, left.Shared.DNS.BlockedResponseTTL, right.Shared.DNS.BlockedResponseTTL)
		add("DNS", "EDNS client subnet", SharedManaged, left.Shared.DNS.EDNSClientSubnet, right.Shared.DNS.EDNSClientSubnet)
		add("DNS", "EDNS custom address", SharedManaged, []any{left.Shared.DNS.EDNSUseCustom, left.Shared.DNS.EDNSCustomIP}, []any{right.Shared.DNS.EDNSUseCustom, right.Shared.DNS.EDNSCustomIP})
		add("DNS", "Disable IPv6", SharedManaged, left.Shared.DNS.DisableIPv6, right.Shared.DNS.DisableIPv6)
		add("DNS", "DNSSEC", SharedManaged, left.Shared.DNS.DNSSECEnabled, right.Shared.DNS.DNSSECEnabled)
		add("DNS", "Cache", SharedManaged, []any{left.Shared.DNS.CacheEnabled, left.Shared.DNS.CacheSize, left.Shared.DNS.CacheTTLMin, left.Shared.DNS.CacheTTLMax, left.Shared.DNS.CacheOptimistic}, []any{right.Shared.DNS.CacheEnabled, right.Shared.DNS.CacheSize, right.Shared.DNS.CacheTTLMin, right.Shared.DNS.CacheTTLMax, right.Shared.DNS.CacheOptimistic})
		add("DNS", "Upstream mode", SharedManaged, left.Shared.DNS.UpstreamMode, right.Shared.DNS.UpstreamMode)
		add("DNS", "Upstream timeout", SharedManaged, left.Shared.DNS.UpstreamTimeout, right.Shared.DNS.UpstreamTimeout)
		add("DNS", "Private reverse and client resolution", SharedManaged, []bool{left.Shared.DNS.UsePrivateReverse, left.Shared.DNS.ResolveClients}, []bool{right.Shared.DNS.UsePrivateReverse, right.Shared.DNS.ResolveClients})
	}
	add("Filtering", "Enabled", SharedManaged, left.Shared.Filtering.Enabled, right.Shared.Filtering.Enabled)
	add("Filtering", "Update interval", SharedManaged, left.Shared.Filtering.UpdateInterval, right.Shared.Filtering.UpdateInterval)
	add("Filtering", "Filter subscriptions", SharedManaged, left.Shared.Filtering.FilterURLs, right.Shared.Filtering.FilterURLs)
	if left.SchemaVersion >= SchemaVersion || right.SchemaVersion >= SchemaVersion {
		add("Filtering", "Allowlist subscriptions", SharedManaged, left.Shared.Filtering.WhitelistURLs, right.Shared.Filtering.WhitelistURLs)
		add("Clients", "Persistent clients", SharedManaged, left.Shared.Clients, right.Shared.Clients)
		add("Rewrites", "DNS rewrites", SharedManaged, []any{left.Shared.RewritesEnabled, left.Shared.Rewrites}, []any{right.Shared.RewritesEnabled, right.Shared.Rewrites})
		add("Services", "Blocked services", SharedManaged, []any{left.Shared.Services.BlockedServiceIDs, left.Shared.Services.BlockedSchedule}, []any{right.Shared.Services.BlockedServiceIDs, right.Shared.Services.BlockedSchedule})
		add("Services", "Safe browsing", SharedManaged, left.Shared.Services.SafeBrowsing, right.Shared.Services.SafeBrowsing)
		add("Services", "Parental control", SharedManaged, left.Shared.Services.ParentalControl, right.Shared.Services.ParentalControl)
		add("Services", "Safe search", SharedManaged, left.Shared.Services.SafeSearch, right.Shared.Services.SafeSearch)
		add("Query log", "Policy", SharedManaged, left.Shared.QueryLog, right.Shared.QueryLog)
		add("Statistics", "Policy", SharedManaged, left.Shared.Statistics, right.Shared.Statistics)
	}
	add("Filtering", "Custom rules", SharedManaged, left.Shared.Filtering.UserRules, right.Shared.Filtering.UserRules)
	add("DNS", "Bind hosts", NodeManaged, left.NodeSpecific.BindHosts, right.NodeSpecific.BindHosts)
	add("DNS", "DNS port", NodeManaged, left.NodeSpecific.DNSPort, right.NodeSpecific.DNSPort)
	if left.SchemaVersion >= SchemaVersion || right.SchemaVersion >= SchemaVersion {
		add("DHCP", "Configuration and static leases", NodeManaged, left.NodeSpecific.DHCP, right.NodeSpecific.DHCP)
	}
	add("Compatibility", "Unsupported areas", UnsupportedField, left.Unsupported, right.Unsupported)
	return diffs
}

func DiffDesired(left, right DesiredDocument) []Difference {
	left, right = CanonicaliseDesired(left), CanonicaliseDesired(right)
	diffs := Diff(Document{Shared: left.Shared, Unsupported: left.Unsupported}, Document{Shared: right.Shared, Unsupported: right.Unsupported})
	nodeIDs := make(map[string]struct{}, len(left.NodeOverrides)+len(right.NodeOverrides))
	for id := range left.NodeOverrides {
		nodeIDs[id] = struct{}{}
	}
	for id := range right.NodeOverrides {
		nodeIDs[id] = struct{}{}
	}
	ordered := make([]string, 0, len(nodeIDs))
	for id := range nodeIDs {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		a, aOK := left.NodeOverrides[id]
		b, bOK := right.NodeOverrides[id]
		if !aOK || !bOK {
			diffs = append(diffs, Difference{Section: "DNS", Field: "Node override", Scope: NodeManaged, Left: valueOrNil(a, aOK), Right: valueOrNil(b, bOK), Summary: "node override differs for " + id})
			continue
		}
		for _, difference := range Diff(Document{NodeSpecific: a}, Document{NodeSpecific: b}) {
			if difference.Scope == NodeManaged {
				difference.Summary += " for " + id
				diffs = append(diffs, difference)
			}
		}
	}
	return diffs
}

func valueOrNil(value NodeSpecific, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func ValidateDesired(document DesiredDocument, nodeIDs []string) []ValidationIssue {
	providedSchemaVersion := document.SchemaVersion
	document = CanonicaliseDesired(document)
	issues := make([]ValidationIssue, 0)
	if providedSchemaVersion != LegacySchemaVersion && providedSchemaVersion != SchemaVersion {
		issues = append(issues, ValidationIssue{Field: "schemaVersion", Message: "must be 1 or 2"})
	}
	allowedIntervals := map[int]bool{0: true, 1: true, 12: true, 24: true, 72: true, 168: true}
	if providedSchemaVersion == LegacySchemaVersion && !allowedIntervals[document.Shared.Filtering.UpdateInterval] {
		issues = append(issues, ValidationIssue{Field: "shared.filtering.updateIntervalHours", Message: "must be 0, 1, 12, 24, 72, or 168"})
	} else if providedSchemaVersion >= SchemaVersion && (document.Shared.Filtering.UpdateInterval < 0 || document.Shared.Filtering.UpdateInterval > 8760) {
		issues = append(issues, ValidationIssue{Field: "shared.filtering.updateIntervalHours", Message: "must be between 0 and 8760"})
	}
	for index, rawURL := range document.Shared.Filtering.FilterURLs {
		if !validHTTPURL(rawURL) {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.filtering.filterUrls[%d]", index), Message: "must be an absolute HTTP or HTTPS URL"})
		}
	}
	for index, rawURL := range document.Shared.Filtering.WhitelistURLs {
		if !validHTTPURL(rawURL) {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.filtering.whitelistUrls[%d]", index), Message: "must be an absolute HTTP or HTTPS URL"})
		}
	}
	if providedSchemaVersion >= SchemaVersion {
		issues = append(issues, validateV2Shared(document.Shared)...)
	}
	enabledDHCP := 0
	for _, nodeID := range nodeIDs {
		override, ok := document.NodeOverrides[nodeID]
		if !ok {
			issues = append(issues, ValidationIssue{Field: "nodeOverrides." + nodeID, Message: "is required for every enabled node"})
			continue
		}
		issues = append(issues, ValidateNodeSpecific("nodeOverrides."+nodeID, override)...)
		if override.DHCP != nil {
			issues = append(issues, validateDHCP("nodeOverrides."+nodeID+".dhcp", *override.DHCP)...)
			if override.DHCP.Enabled {
				enabledDHCP++
			}
		}
	}
	if enabledDHCP > 1 {
		issues = append(issues, ValidationIssue{Field: "nodeOverrides", Message: "DHCP may be enabled on at most one node"})
	}
	return issues
}

func validHTTPURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func validateV2Shared(shared Shared) []ValidationIssue {
	issues := []ValidationIssue{}
	if shared.DNS.RateLimit < 0 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.rateLimit", Message: "must not be negative"})
	}
	if shared.DNS.BlockedResponseTTL < 0 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.blockedResponseTtl", Message: "must not be negative"})
	}
	if shared.DNS.RateLimitIPv4 < 0 || shared.DNS.RateLimitIPv4 > 32 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.rateLimitSubnetLengthIpv4", Message: "must be between 0 and 32"})
	}
	if shared.DNS.RateLimitIPv6 < 0 || shared.DNS.RateLimitIPv6 > 128 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.rateLimitSubnetLengthIpv6", Message: "must be between 0 and 128"})
	}
	allowedModes := map[string]bool{"": true, "default": true, "refused": true, "nxdomain": true, "null_ip": true, "custom_ip": true}
	if !allowedModes[shared.DNS.BlockingMode] {
		issues = append(issues, ValidationIssue{Field: "shared.dns.blockingMode", Message: "is not supported"})
	}
	if shared.DNS.BlockingIPv4 != "" {
		address, err := netip.ParseAddr(shared.DNS.BlockingIPv4)
		if err != nil || !address.Is4() {
			issues = append(issues, ValidationIssue{Field: "shared.dns.blockingIpv4", Message: "must be an IPv4 address"})
		}
	}
	if shared.DNS.BlockingIPv6 != "" {
		address, err := netip.ParseAddr(shared.DNS.BlockingIPv6)
		if err != nil || !address.Is6() {
			issues = append(issues, ValidationIssue{Field: "shared.dns.blockingIpv6", Message: "must be an IPv6 address"})
		}
	}
	allowedUpstreamModes := map[string]bool{"": true, "load_balance": true, "fastest_addr": true, "parallel": true}
	if !allowedUpstreamModes[shared.DNS.UpstreamMode] {
		issues = append(issues, ValidationIssue{Field: "shared.dns.upstreamMode", Message: "is not supported"})
	}
	for index, address := range shared.DNS.RateLimitAllowlist {
		if _, err := netip.ParsePrefix(address); err != nil {
			if _, addressErr := netip.ParseAddr(address); addressErr != nil {
				issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.dns.rateLimitAllowlist[%d]", index), Message: "must be an IP address or prefix"})
			}
		}
	}
	if shared.DNS.EDNSUseCustom && !validAddress(shared.DNS.EDNSCustomIP) {
		issues = append(issues, ValidationIssue{Field: "shared.dns.ednsCustomIp", Message: "must be an IP address when custom EDNS is enabled"})
	}
	if shared.DNS.CacheSize < 0 || shared.DNS.CacheTTLMin < 0 || shared.DNS.CacheTTLMax < 0 || (shared.DNS.CacheTTLMax > 0 && shared.DNS.CacheTTLMin > shared.DNS.CacheTTLMax) {
		issues = append(issues, ValidationIssue{Field: "shared.dns.cache", Message: "cache size and TTL values must be non-negative and the minimum must not exceed the maximum"})
	}
	if shared.DNS.CacheEnabled && shared.DNS.CacheSize <= 0 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.cacheSize", Message: "must be positive when the cache is enabled"})
	}
	if shared.DNS.UpstreamTimeout < 0 {
		issues = append(issues, ValidationIssue{Field: "shared.dns.upstreamTimeoutSeconds", Message: "must not be negative"})
	}
	clientNames, clientIDs := map[string]bool{}, map[string]bool{}
	for index, client := range shared.Clients {
		key := strings.ToLower(client.Name)
		if client.Name == "" || clientNames[key] {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.clients[%d].name", index), Message: "must be non-empty and unique"})
		}
		clientNames[key] = true
		if len(client.IDs) == 0 {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.clients[%d].ids", index), Message: "must contain at least one identifier"})
		}
		for _, id := range client.IDs {
			idKey := strings.ToLower(id)
			if clientIDs[idKey] {
				issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.clients[%d].ids", index), Message: "identifiers must be unique across clients"})
			}
			clientIDs[idKey] = true
		}
		if client.UpstreamsCacheSize < 0 {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.clients[%d].upstreamsCacheSize", index), Message: "must not be negative"})
		}
		issues = append(issues, validateSchedule(fmt.Sprintf("shared.clients[%d].blockedServicesSchedule", index), client.BlockedServicesSchedule)...)
	}
	seenRewrites := map[string]bool{}
	for index, rewrite := range shared.Rewrites {
		key := strings.ToLower(rewrite.Domain) + "\x00" + strings.ToLower(rewrite.Answer)
		if rewrite.Domain == "" || rewrite.Answer == "" || seenRewrites[key] {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("shared.rewrites[%d]", index), Message: "domain and answer must be non-empty and the pair must be unique"})
		}
		seenRewrites[key] = true
	}
	issues = append(issues, validateSchedule("shared.services.blockedSchedule", shared.Services.BlockedSchedule)...)
	for field, value := range map[string]int64{"shared.queryLog.intervalMillis": shared.QueryLog.IntervalMillis, "shared.statistics.intervalMillis": shared.Statistics.IntervalMillis} {
		if value != 0 && (value < 3_600_000 || value > 31_536_000_000) {
			issues = append(issues, ValidationIssue{Field: field, Message: "must be between one hour and one year in milliseconds"})
		}
	}
	if shared.QueryLog.Enabled && shared.QueryLog.IntervalMillis == 0 {
		issues = append(issues, ValidationIssue{Field: "shared.queryLog.intervalMillis", Message: "must be set when query logging is enabled"})
	}
	if shared.Statistics.Enabled && shared.Statistics.IntervalMillis == 0 {
		issues = append(issues, ValidationIssue{Field: "shared.statistics.intervalMillis", Message: "must be set when statistics are enabled"})
	}
	return issues
}

func validateSchedule(prefix string, schedule Schedule) []ValidationIssue {
	issues := []ValidationIssue{}
	if schedule.TimeZone != "" && schedule.TimeZone != "Local" {
		if _, err := time.LoadLocation(schedule.TimeZone); err != nil {
			issues = append(issues, ValidationIssue{Field: prefix + ".timeZone", Message: "must be Local or an IANA time zone"})
		}
	}
	allowed := map[string]bool{"sun": true, "mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true}
	for day, period := range schedule.Days {
		if !allowed[day] || period.Start < 0 || period.End > 86_400_000 || period.Start >= period.End {
			issues = append(issues, ValidationIssue{Field: prefix + ".days." + day, Message: "must be a valid day with start before end within 24 hours"})
		}
	}
	return issues
}

func validateDHCP(prefix string, config DHCPConfig) []ValidationIssue {
	issues := []ValidationIssue{}
	if config.Enabled {
		if strings.TrimSpace(config.InterfaceName) == "" {
			issues = append(issues, ValidationIssue{Field: prefix + ".interfaceName", Message: "is required when DHCP is enabled"})
		}
		if config.IPv4.LeaseDuration <= 0 {
			issues = append(issues, ValidationIssue{Field: prefix + ".ipv4.leaseDurationSeconds", Message: "must be positive when DHCP is enabled"})
		}
	}
	if config.Enabled || config.IPv4.Gateway != "" || config.IPv4.SubnetMask != "" || config.IPv4.RangeStart != "" || config.IPv4.RangeEnd != "" {
		gateway, gatewayErr := netip.ParseAddr(config.IPv4.Gateway)
		start, startErr := netip.ParseAddr(config.IPv4.RangeStart)
		end, endErr := netip.ParseAddr(config.IPv4.RangeEnd)
		maskIP := net.ParseIP(config.IPv4.SubnetMask).To4()
		ones, bits := 0, 0
		if maskIP != nil {
			ones, bits = net.IPMask(maskIP).Size()
		}
		if gatewayErr != nil || !gateway.Is4() || startErr != nil || !start.Is4() || endErr != nil || !end.Is4() || bits != 32 {
			issues = append(issues, ValidationIssue{Field: prefix + ".ipv4", Message: "gateway, contiguous subnet mask, range start, and range end must be valid IPv4 values"})
		} else {
			subnet := netip.PrefixFrom(gateway, ones).Masked()
			if !subnet.Contains(start) || !subnet.Contains(end) {
				issues = append(issues, ValidationIssue{Field: prefix + ".ipv4", Message: "range start and end must be within the gateway subnet"})
			}
			if end.Less(start) {
				issues = append(issues, ValidationIssue{Field: prefix + ".ipv4.rangeEnd", Message: "must not be before range start"})
			}
		}
	}
	if config.IPv6.RangeStart != "" {
		start, err := netip.ParseAddr(config.IPv6.RangeStart)
		if err != nil || !start.Is6() {
			issues = append(issues, ValidationIssue{Field: prefix + ".ipv6.rangeStart", Message: "must be an IPv6 address"})
		}
		if config.IPv6.LeaseDuration <= 0 {
			issues = append(issues, ValidationIssue{Field: prefix + ".ipv6.leaseDurationSeconds", Message: "must be positive when an IPv6 range is configured"})
		}
	}
	seenMACs, seenIPs := map[string]bool{}, map[string]bool{}
	for index, lease := range config.StaticLeases {
		mac, macErr := net.ParseMAC(strings.TrimSpace(lease.MAC))
		ip, ipErr := netip.ParseAddr(strings.TrimSpace(lease.IP))
		hostname := strings.TrimSpace(lease.Hostname)
		hostnameValid := validDHCPHostname(hostname)
		if macErr != nil {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s.staticLeases[%d].mac", prefix, index), Message: "must be a valid six-byte MAC address"})
		}
		if ipErr != nil {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s.staticLeases[%d].ip", prefix, index), Message: "must be a valid IP address"})
		}
		if !hostnameValid {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s.staticLeases[%d].hostname", prefix, index), Message: "must contain valid hostname labels"})
		}
		if macErr != nil || ipErr != nil || !hostnameValid {
			continue
		}
		macKey, ipKey := mac.String(), ip.String()
		if seenMACs[macKey] || seenIPs[ipKey] {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s.staticLeases[%d]", prefix, index), Message: "MAC and IP must be unique within the node"})
		}
		seenMACs[macKey], seenIPs[ipKey] = true, true
	}
	return issues
}

func validDHCPHostname(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validAddress(value string) bool {
	_, err := netip.ParseAddr(value)
	return err == nil
}

// ValidateNodeSpecific validates the listener identity retained for a node.
// These values are observed and verified by the controller, not written.
func ValidateNodeSpecific(fieldPrefix string, listener NodeSpecific) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if listener.DNSPort < 1 || listener.DNSPort > 65535 {
		issues = append(issues, ValidationIssue{Field: fieldPrefix + ".dnsPort", Message: "must be between 1 and 65535"})
	}
	if len(listener.BindHosts) == 0 {
		issues = append(issues, ValidationIssue{Field: fieldPrefix + ".bindHosts", Message: "must include at least one address"})
	}
	for index, host := range listener.BindHosts {
		if _, err := netip.ParseAddr(host); err != nil {
			issues = append(issues, ValidationIssue{Field: fmt.Sprintf("%s.bindHosts[%d]", fieldPrefix, index), Message: "must be an IP address"})
		}
	}
	return issues
}

func cleanOrdered(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func cleanSet(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	if result == nil {
		return []string{}
	}
	return result
}

func canonicalSchedule(schedule Schedule) Schedule {
	schedule.TimeZone = strings.TrimSpace(schedule.TimeZone)
	if schedule.Days == nil {
		schedule.Days = map[string]DayRange{}
	}
	return schedule
}

func canonicaliseDHCP(config *DHCPConfig) {
	config.InterfaceName = strings.TrimSpace(config.InterfaceName)
	for index := range config.StaticLeases {
		config.StaticLeases[index].MAC = strings.ToLower(strings.TrimSpace(config.StaticLeases[index].MAC))
		config.StaticLeases[index].IP = strings.TrimSpace(config.StaticLeases[index].IP)
		config.StaticLeases[index].Hostname = strings.TrimSpace(config.StaticLeases[index].Hostname)
	}
	sort.Slice(config.StaticLeases, func(i, j int) bool { return config.StaticLeases[i].MAC < config.StaticLeases[j].MAC })
}
