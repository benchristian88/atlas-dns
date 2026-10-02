package adguardcompat

import (
	"testing"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

func TestCompatibilityAndAPIGeneration(t *testing.T) {
	for _, version := range []string{
		"v0.107.78", "0.107.78", "v0.107.79", "v0.107.80", "v0.107.79-rc.1",
		"v0.108.0-b.90", "v0.108.0-b.91", "v0.108.0", "v0.200.4",
		"v1.0.0-a.1", "v1.0.0-b.1", "v1.0.0-rc.1", "v1.0.0", "v1.0.1",
		"v1.1.0", "v1.20.4", "v1.25.7", "v1.27.12", "v1.999.999",
		"v1.2.3-beta.2+build.001", " v0.107.78+build.123 ",
		"v1.999999999999999999999999999999.999999999999999999999999999999",
		"v0.107.999999999999999999999999999999", "v0.999999999999999999999999999999.0",
	} {
		t.Run(version, func(t *testing.T) {
			if got := Compatibility(version); got != domain.CompatibilitySupported {
				t.Errorf("Compatibility(%q) = %s", version, got)
			}
			if got := Generation(version); got != APIGenerationLegacyControl {
				t.Errorf("Generation(%q) = %s", version, got)
			}
		})
	}
	for _, version := range []string{"v0.106.3", "v0.107.77", "v0.107.78-a.1", "v0.107.78-rc.1+build.1"} {
		if got := Compatibility(version); got != domain.CompatibilityUnsupported {
			t.Errorf("Compatibility(%q) = %s; prereleases must stay below the stable minimum", version, got)
		}
		if Generation(version) != APIGenerationLegacyControl || IsProvisionallyCompatible(version) {
			t.Errorf("old version %q must have a known generation but remain ineligible", version)
		}
	}
	for _, version := range []string{"v2.0.0", "v2.0.0-b.1", "v3.1.2", "garbage", ""} {
		if Compatibility(version) != domain.CompatibilityUnknown || Generation(version) != APIGenerationUnknown || IsProvisionallyCompatible(version) {
			t.Errorf("unrecognized version %q must remain unknown", version)
		}
	}
}

func TestMalformedSemanticVersionsRemainUnknown(t *testing.T) {
	for _, version := range []string{
		"v1.0", "v1.0.0.1", "v1.0.0-", "v1.0.0+", "v1.0.0-beta..1", "v1.0.0+build..1",
		"v1.0.0-01", "v1.0.0-beta.01", "v01.0.0", "v1.00.0", "v1.0.00", "v-1.0.0",
		"v1.+1.0", "v1.0.0/beta", "v1.0.0-beta_1", "v1.0.0 beta", "vv1.0.0",
		"v0.107.78\nextra",
	} {
		if Compatibility(version) != domain.CompatibilityUnknown || Generation(version) != APIGenerationUnknown {
			t.Errorf("malformed version %q was recognized", version)
		}
	}
}

func TestProvisionalCompatibility(t *testing.T) {
	for _, version := range []string{"v0.107.78", "v0.107.79", "v0.107.79+build.1"} {
		if IsProvisionallyCompatible(version) {
			t.Errorf("release-tested version %q was provisional", version)
		}
	}
	for _, version := range []string{"v0.107.80", "v0.107.79-rc.1", "v0.108.0-b.91", "v1.0.0-b.1", "v1.0.0", "v1.27.12"} {
		if !IsProvisionallyCompatible(version) {
			t.Errorf("eligible untested version %q was not provisional", version)
		}
	}
}
