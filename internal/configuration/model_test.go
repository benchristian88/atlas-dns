package configuration

import (
	"strings"
	"testing"
)

func TestEquivalentDocumentsHaveSameHash(t *testing.T) {
	a := Document{Shared: Shared{DNS: DNS{BootstrapDNS: []string{"1.1.1.1", "9.9.9.9"}}, Filtering: Filtering{FilterURLs: []string{"https://b", "https://a"}}}, NodeSpecific: NodeSpecific{BindHosts: []string{"127.0.0.1", "0.0.0.0"}}}
	b := Document{Shared: Shared{DNS: DNS{BootstrapDNS: []string{"9.9.9.9", "1.1.1.1"}}, Filtering: Filtering{FilterURLs: []string{"https://a", "https://b"}}}, NodeSpecific: NodeSpecific{BindHosts: []string{"0.0.0.0", "127.0.0.1"}}}
	_, ah, err := Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	_, bh, err := Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if ah != bh {
		t.Fatalf("equivalent documents differ: %s != %s", ah, bh)
	}
}

func TestDiffPreservesOrderedUpstreamsAndGroupsScope(t *testing.T) {
	a := Document{Shared: Shared{DNS: DNS{UpstreamDNS: []string{"a", "b"}}}}
	b := Document{Shared: Shared{DNS: DNS{UpstreamDNS: []string{"b", "a"}}}}
	diffs := Diff(a, b)
	if len(diffs) != 1 || diffs[0].Section != "DNS" || diffs[0].Scope != SharedManaged {
		t.Fatalf("unexpected differences: %#v", diffs)
	}
}

func TestDesiredDocumentBuildsNodeEffectiveState(t *testing.T) {
	desired := DesiredDocument{
		SchemaVersion: SchemaVersion,
		Shared:        Shared{DNS: DNS{UpstreamDNS: []string{"https://dns.example/dns-query"}}, Filtering: Filtering{UpdateInterval: 24}},
		NodeOverrides: map[string]NodeSpecific{
			"node-a": {BindHosts: []string{"192.0.2.10"}, DNSPort: 53},
			"node-b": {BindHosts: []string{"192.0.2.11"}, DNSPort: 53},
		},
	}
	a, err := Effective(desired, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Effective(desired, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	if a.NodeSpecific.BindHosts[0] == b.NodeSpecific.BindHosts[0] || a.Shared.DNS.UpstreamDNS[0] != b.Shared.DNS.UpstreamDNS[0] {
		t.Fatalf("effective state did not preserve shared policy and node identity: %#v %#v", a, b)
	}
}

func TestValidateDesiredRequiresEveryNodeOverrideAndValidListener(t *testing.T) {
	desired := DesiredDocument{SchemaVersion: SchemaVersion, Shared: Shared{Filtering: Filtering{UpdateInterval: 24}}, NodeOverrides: map[string]NodeSpecific{
		"node-a": {BindHosts: []string{"not-an-ip"}, DNSPort: 0},
	}}
	issues := ValidateDesired(desired, []string{"node-a", "node-b"})
	if len(issues) != 3 {
		t.Fatalf("issues = %#v, want listener address, port, and missing override", issues)
	}
}

func TestLegacyRecordConversionIsSchemaV2AndNotDeployable(t *testing.T) {
	converted := ConvertLegacyDesired(DesiredDocument{SchemaVersion: LegacySchemaVersion, Shared: Shared{Filtering: Filtering{UpdateInterval: 24}}})
	if converted.SchemaVersion != SchemaVersion || !IsLegacyConverted(converted.Unsupported) {
		t.Fatalf("converted = %#v", converted)
	}
	if issues := ValidateDesired(converted, nil); len(issues) != 1 || !strings.Contains(issues[0].Message, "legacy") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateDesiredRejectsUnsafeDHCPRole(t *testing.T) {
	dhcp := &DHCPConfig{Enabled: true, InterfaceName: "eth0", IPv4: DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.2.10", RangeEnd: "192.0.2.20", LeaseDuration: 3600}}
	document := DesiredDocument{SchemaVersion: SchemaVersion, NodeOverrides: map[string]NodeSpecific{
		"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53, DHCP: dhcp},
		"node-b": {BindHosts: []string{"192.0.2.3"}, DNSPort: 53, DHCP: dhcp},
	}}
	issues := ValidateDesired(document, []string{"node-a", "node-b"})
	found := false
	for _, issue := range issues {
		if issue.Field == "nodeOverrides" && strings.Contains(issue.Message, "at most one") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing single-active DHCP validation: %#v", issues)
	}
}

func TestValidateDesiredRejectsInvalidDHCPNetworkAndDuplicateLeases(t *testing.T) {
	document := DesiredDocument{SchemaVersion: SchemaVersion, NodeOverrides: map[string]NodeSpecific{
		"node-a": {
			BindHosts: []string{"192.0.2.2"},
			DNSPort:   53,
			DHCP: &DHCPConfig{
				Enabled:       true,
				InterfaceName: "eth0",
				IPv4:          DHCPIPv4{Gateway: "192.0.2.1", SubnetMask: "255.255.255.0", RangeStart: "192.0.3.20", RangeEnd: "192.0.2.10", LeaseDuration: 3600},
				IPv6:          DHCPIPv6{RangeStart: "192.0.2.10", LeaseDuration: 0},
				StaticLeases: []DHCPStaticLease{
					{MAC: "00:11:22:33:44:55", IP: "192.0.2.30", Hostname: "one"},
					{MAC: "00:11:22:33:44:55", IP: "192.0.2.31", Hostname: "two"},
				},
			},
		},
	}}
	issues := ValidateDesired(document, []string{"node-a"})
	for _, expected := range []string{"gateway subnet", "before range start", "must be an IPv6 address", "must be positive", "must be unique"} {
		found := false
		for _, issue := range issues {
			if strings.Contains(issue.Message, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q validation in %#v", expected, issues)
		}
	}
}

func TestValidateDesiredRejectsInvalidStaticLeaseIdentity(t *testing.T) {
	document := DesiredDocument{SchemaVersion: SchemaVersion, NodeOverrides: map[string]NodeSpecific{
		"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53, DHCP: &DHCPConfig{StaticLeases: []DHCPStaticLease{{MAC: "not-a-mac", IP: "not-an-ip", Hostname: "bad_host"}}}},
	}}
	issues := ValidateDesired(document, []string{"node-a"})
	for _, suffix := range []string{".mac", ".ip", ".hostname"} {
		found := false
		for _, issue := range issues {
			if strings.HasSuffix(issue.Field, suffix) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s validation in %#v", suffix, issues)
		}
	}
}

func TestValidateDesiredRequiresEnabledTelemetryIntervals(t *testing.T) {
	document := DesiredDocument{
		SchemaVersion: SchemaVersion,
		Shared:        Shared{QueryLog: QueryLogPolicy{Enabled: true}, Statistics: StatisticsPolicy{Enabled: true}},
		NodeOverrides: map[string]NodeSpecific{"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53}},
	}
	issues := ValidateDesired(document, []string{"node-a"})
	found := 0
	for _, issue := range issues {
		if strings.Contains(issue.Field, "intervalMillis") && strings.Contains(issue.Message, "must be set") {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("enabled telemetry interval issues = %#v", issues)
	}
}

func TestValidateDesiredRejectsInvalidV2DNSAddresses(t *testing.T) {
	document := DesiredDocument{
		SchemaVersion: SchemaVersion,
		Shared: Shared{DNS: DNS{
			BlockedResponseTTL: -1,
			BlockingIPv4:       "2001:db8::1",
			BlockingIPv6:       "192.0.2.1",
		}},
		NodeOverrides: map[string]NodeSpecific{"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53}},
	}
	issues := ValidateDesired(document, []string{"node-a"})
	for _, field := range []string{"shared.dns.blockedResponseTtl", "shared.dns.blockingIpv4", "shared.dns.blockingIpv6"} {
		found := false
		for _, issue := range issues {
			if issue.Field == field {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s validation in %#v", field, issues)
		}
	}
}

func TestValidateDesiredAllowsCurrentV2FilterIntervalRange(t *testing.T) {
	document := DesiredDocument{
		SchemaVersion: SchemaVersion,
		Shared: Shared{
			DNS:       DNS{CacheEnabled: true, CacheSize: 4_194_304},
			Filtering: Filtering{UpdateInterval: 48},
		},
		NodeOverrides: map[string]NodeSpecific{"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53}},
	}
	if issues := ValidateDesired(document, []string{"node-a"}); len(issues) != 0 {
		t.Fatalf("current schema-v2 interval rejected: %#v", issues)
	}
}

func TestValidateDesiredRejectsNonPortableAndCredentialedFilterURLs(t *testing.T) {
	document := DesiredDocument{
		SchemaVersion: SchemaVersion,
		Shared: Shared{Filtering: Filtering{FilterURLs: []string{
			"/opt/adguard/list.txt",
			"https://user:secret@filters.test/list.txt",
		}}},
		NodeOverrides: map[string]NodeSpecific{"node-a": {BindHosts: []string{"192.0.2.2"}, DNSPort: 53}},
	}
	issues := ValidateDesired(document, []string{"node-a"})
	filterIssues := 0
	for _, issue := range issues {
		if strings.HasPrefix(issue.Field, "shared.filtering.filterUrls") {
			filterIssues++
		}
	}
	if filterIssues != 2 {
		t.Fatalf("unsupported URLs were not rejected: %#v", issues)
	}
}
