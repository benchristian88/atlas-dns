package domain

import "testing"

func TestNormaliseNodeURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		policy  CertificatePolicy
		want    string
		wantErr bool
	}{
		{name: "https", value: "https://node.example.test/", policy: CertificateSystemTrust, want: "https://node.example.test"},
		{name: "http explicit", value: "http://192.0.2.2", policy: CertificateInsecureHTTP, want: "http://192.0.2.2"},
		{name: "http implicit", value: "http://192.0.2.2", policy: CertificateSystemTrust, wantErr: true},
		{name: "credentials", value: "https://user:password@node.test", policy: CertificateSystemTrust, wantErr: true},
		{name: "query", value: "https://node.test?password=value", policy: CertificateSystemTrust, wantErr: true},
		{name: "fragment", value: "https://node.test#status", policy: CertificateSystemTrust, wantErr: true},
		{name: "file scheme", value: "file:///etc/passwd", policy: CertificateSystemTrust, wantErr: true},
		{name: "gopher scheme", value: "gopher://127.0.0.1:70", policy: CertificateSystemTrust, wantErr: true},
		{name: "ftp scheme", value: "ftp://node.test/config", policy: CertificateSystemTrust, wantErr: true},
		{name: "scheme relative", value: "//node.test", policy: CertificateSystemTrust, wantErr: true},
		{name: "empty host", value: "https:///control/status", policy: CertificateSystemTrust, wantErr: true},
		{name: "loopback https", value: "https://127.0.0.1:8443", policy: CertificateSystemTrust, want: "https://127.0.0.1:8443"},
		{name: "ipv6 https", value: "https://[::1]:8443/", policy: CertificateSystemTrust, want: "https://[::1]:8443"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormaliseNodeURL(test.value, test.policy)
			if (err != nil) != test.wantErr {
				t.Fatalf("NormaliseNodeURL() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("NormaliseNodeURL() = %q, want %q", got, test.want)
			}
		})
	}
}
