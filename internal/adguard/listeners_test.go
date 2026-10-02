package adguard

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
)

func TestValidateListenerStatusProtocols(t *testing.T) {
	for _, test := range []struct {
		name      string
		addresses []string
		valid     bool
	}{
		{"IPv4", []string{"192.168.1.100"}, true},
		{"IPv4 and IPv6", []string{"0.0.0.0", "::"}, true},
		{"DoH", []string{"192.168.1.100", "https://server.localdomain/dns-query"}, true},
		{"DoT", []string{"192.168.1.100", "tls://server.localdomain:853"}, true},
		{"DoQ", []string{"192.168.1.100", "quic://server.localdomain:853"}, true},
		{"all encrypted", []string{"192.168.1.100", "https://server.localdomain/dns-query", "tls://server.localdomain:853", "quic://server.localdomain:853"}, true},
		{"IPv6 encrypted", []string{"::", "https://[2001:db8::1]/dns-query", "tls://[2001:db8::1]:853", "quic://[2001:db8::1]:853"}, true},
		{"whitespace and scheme case", []string{" 192.168.1.100 ", " HTTPS://server.localdomain/dns-query ", "TLS://server.localdomain:853", "QuIc://server.localdomain:853"}, true},
		{"endpoint without explicit port", []string{"::", "tls://server.localdomain", "quic://server.localdomain"}, true},
		{"garbage", []string{"192.168.1.100", "definitely-not-an-address"}, false},
		{"unknown scheme", []string{"192.168.1.100", "ftp://server.localdomain"}, false},
		{"garbage scheme", []string{"192.168.1.100", "garbage://"}, false},
		{"broken URI", []string{"192.168.1.100", "://broken"}, false},
		{"URI only", []string{"https://server.localdomain/dns-query", "tls://server.localdomain:853"}, false},
		{"empty", []string{}, false},
		{"blank", []string{"192.168.1.100", " "}, false},
		{"empty host", []string{"192.168.1.100", "https:///dns-query"}, false},
		{"empty host with port", []string{"192.168.1.100", "tls://:853"}, false},
		{"opaque URI", []string{"192.168.1.100", "tls:server.localdomain"}, false},
		{"invalid port", []string{"192.168.1.100", "quic://server.localdomain:abc"}, false},
		{"out of range port", []string{"192.168.1.100", "tls://server.localdomain:65536"}, false},
		{"zero port", []string{"192.168.1.100", "tls://server.localdomain:0"}, false},
		{"empty port", []string{"192.168.1.100", "tls://server.localdomain:"}, false},
		{"userinfo", []string{"192.168.1.100", "https://user:secret@server.localdomain/dns-query"}, false},
		{"invalid escape", []string{"192.168.1.100", "https://server.localdomain/%zz"}, false},
		{"invalid brackets", []string{"192.168.1.100", "tls://[not-ip]:853"}, false},
		{"unbracketed IPv6", []string{"192.168.1.100", "tls://2001:db8::1:853"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			enabled, duration := true, int64(0)
			err := validateListenerStatus(statusResponse{DNSAddresses: test.addresses, DNSPort: 53, ProtectionEnabled: &enabled, ProtectionDisabledDurationMS: &duration})
			if test.valid && err != nil {
				t.Fatalf("valid listener rejected: %v", err)
			}
			if !test.valid {
				assertDomainErrorKind(t, err, domain.ErrorNodeResponse)
			}
		})
	}
}

func TestValidateListenerStatusPreservesRequiredSemantics(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*statusResponse)
	}{
		{"zero port", func(s *statusResponse) { s.DNSPort = 0 }},
		{"high port", func(s *statusResponse) { s.DNSPort = 65536 }},
		{"missing protection", func(s *statusResponse) { s.ProtectionEnabled = nil }},
		{"missing duration", func(s *statusResponse) { s.ProtectionDisabledDurationMS = nil }},
		{"negative duration", func(s *statusResponse) { *s.ProtectionDisabledDurationMS = -1 }},
		{"contradictory protection", func(s *statusResponse) { *s.ProtectionDisabledDurationMS = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			enabled, duration := true, int64(0)
			status := statusResponse{DNSAddresses: []string{"192.168.1.100", "https://server.localdomain/dns-query"}, DNSPort: 53, ProtectionEnabled: &enabled, ProtectionDisabledDurationMS: &duration}
			test.mutate(&status)
			assertDomainErrorKind(t, validateListenerStatus(status), domain.ErrorNodeResponse)
		})
	}
}

func TestPlainDNSBindHosts(t *testing.T) {
	for _, test := range []struct{ addresses, want []string }{
		{[]string{"192.168.1.100", "https://server.localdomain/dns-query", "tls://server.localdomain:853", "quic://server.localdomain:853"}, []string{"192.168.1.100"}},
		{[]string{"0.0.0.0", "::", "https://dns.example/dns-query"}, []string{"0.0.0.0", "::"}},
		{[]string{" 2001:0db8:0:0::1 ", " 192.168.1.100 "}, []string{"2001:db8::1", "192.168.1.100"}},
		{[]string{"hello", "garbage://", "https://dns.example/dns-query"}, []string{}},
		{nil, []string{}},
	} {
		original := append([]string(nil), test.addresses...)
		got := plainDNSBindHosts(test.addresses)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("plainDNSBindHosts(%v) = %v, want %v", test.addresses, got, test.want)
		}
		for _, host := range got {
			if _, err := netip.ParseAddr(host); err != nil {
				t.Fatalf("non-IP entered bind hosts: %q", host)
			}
		}
		if len(original) > 0 && !reflect.DeepEqual(original, test.addresses) {
			t.Fatal("input addresses were mutated")
		}
	}
}

func TestConfigurationDocumentExcludesEncryptedListeners(t *testing.T) {
	var status statusResponse
	var dns dnsInfoResponse
	readFixture(t, "testdata/v1.0.0/status.json", &status)
	readFixture(t, "testdata/v1.0.0/dns_info.json", &dns)
	document := configurationDocument(status.Version, status, dns, filterStatusResponse{})
	if !reflect.DeepEqual(document.NodeSpecific.BindHosts, []string{"192.168.1.100"}) {
		t.Fatalf("encrypted endpoints leaked into document: %v", document.NodeSpecific.BindHosts)
	}
	if issues := configuration.ValidateNodeSpecific("nodeSpecific", document.NodeSpecific); len(issues) > 0 {
		t.Fatalf("import listener validation failed: %v", issues)
	}
	const nodeID = "22222222-2222-4222-8222-222222222222"
	if issues := configuration.ValidateDesired(configuration.DesiredFromObservation(nodeID, document), []string{nodeID}); len(issues) > 0 {
		t.Fatalf("canonical validation failed: %v", issues)
	}
}
