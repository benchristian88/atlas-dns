package integration_test

import "fmt"

// validAdGuardStatusResponse is the shared integration representation of the
// status semantics Atlas requires from supported AdGuard Home 0.107 nodes.
func validAdGuardStatusResponse(version string) string {
	return fmt.Sprintf(`{"version":%q,"running":true,"dns_addresses":["0.0.0.0"],"dns_port":53,"http_port":3000,"protection_enabled":true,"protection_disabled_duration":0,"language":"en"}`, version)
}
