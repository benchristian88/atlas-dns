// Package adguardcompat classifies product versions independently of the
// AdGuard Home adapter and its consumers, avoiding a querylog/adapter cycle.
package adguardcompat

import (
	"regexp"
	"strings"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type APIGeneration string

const (
	APIGenerationUnknown       APIGeneration = "unknown"
	APIGenerationLegacyControl APIGeneration = "legacy_control"
)

// This is the SemVer 2.0 grammar, with AdGuard's optional leading v. Numeric
// prerelease identifiers may not have leading zeros; build identifiers may.
var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type productVersion struct {
	major, minor, patch string
	prerelease          string
}

func parse(version string) (productVersion, bool) {
	parts := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if parts == nil {
		return productVersion{}, false
	}
	return productVersion{parts[1], parts[2], parts[3], parts[4]}, true
}

// The grammar ensures canonical decimal components. Comparing their lengths
// and digits avoids overflow and any artificial minor/patch ceiling.
func decimalLess(left, right string) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return left < right
}

// Generation identifies the known /control adapter, not a product minor
// release. A future api_v1 adapter requires an explicit policy update.
func Generation(version string) APIGeneration {
	v, ok := parse(version)
	if ok && (v.major == "0" || v.major == "1") {
		return APIGenerationLegacyControl
	}
	return APIGenerationUnknown
}

// Compatibility only grants eligibility to try the adapter. HTTP, typed JSON,
// and semantic response validation remain mandatory for every operation.
func Compatibility(version string) domain.Compatibility {
	v, ok := parse(version)
	if !ok || (v.major != "0" && v.major != "1") {
		return domain.CompatibilityUnknown
	}
	if v.major == "0" && (decimalLess(v.minor, "107") || (v.minor == "107" &&
		(decimalLess(v.patch, "78") || (v.patch == "78" && v.prerelease != "")))) {
		return domain.CompatibilityUnsupported
	}
	return domain.CompatibilitySupported
}

// IsProvisionallyCompatible preserves the distinction between release-tested
// stable patches and eligible versions requiring the same endpoint validation.
func IsProvisionallyCompatible(version string) bool {
	if Compatibility(version) != domain.CompatibilitySupported {
		return false
	}
	v, _ := parse(version)
	return !(v.major == "0" && v.minor == "107" && (v.patch == "78" || v.patch == "79") && v.prerelease == "")
}
