package adguard

import (
	"reflect"
	"testing"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

// AdGuard Home reports every plain listener as host:port whenever dns.port is
// not 53 (internal/home/control.go, appendDNSAddrs). Routers that keep dnsmasq
// on 53 and redirect to AdGuard on another port (GL.iNet uses 3053) report
// exactly this shape.
func TestValidateListenerStatusNonDefaultPort(t *testing.T) {
	addresses := []string{"127.0.0.1:3053", "[::1]:3053", "[fe80::1%eth0]:3053", "192.168.1.1:3053", "[2001:db8::1]:3053", "https://dns.example/dns-query"}
	enabled, duration := true, int64(0)
	if err := validateListenerStatus(statusResponse{DNSAddresses: addresses, DNSPort: 3053, ProtectionEnabled: &enabled, ProtectionDisabledDurationMS: &duration}); err != nil {
		t.Fatalf("valid non-default-port listener rejected: %v", err)
	}
	t.Run("port disagrees with dns_port", func(t *testing.T) {
		err := validateListenerStatus(statusResponse{DNSAddresses: []string{"192.168.1.1:5353"}, DNSPort: 3053, ProtectionEnabled: &enabled, ProtectionDisabledDurationMS: &duration})
		assertDomainErrorKind(t, err, domain.ErrorNodeResponse)
	})
	t.Run("bare address with default port still valid", func(t *testing.T) {
		if err := validateListenerStatus(statusResponse{DNSAddresses: []string{"192.168.1.1"}, DNSPort: 53, ProtectionEnabled: &enabled, ProtectionDisabledDurationMS: &duration}); err != nil {
			t.Fatalf("valid listener rejected: %v", err)
		}
	})
}

func TestPlainDNSBindHostsStripsPort(t *testing.T) {
	got := plainDNSBindHosts([]string{"127.0.0.1:3053", "[::1]:3053", "[fe80::1%eth0]:3053", "192.168.1.1:3053", "[2001:db8::1]:3053", "https://dns.example/dns-query"})
	want := []string{"127.0.0.1", "::1", "fe80::1%eth0", "192.168.1.1", "2001:db8::1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plainDNSBindHosts() = %v, want %v", got, want)
	}
}
