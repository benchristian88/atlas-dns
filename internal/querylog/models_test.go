package querylog

import (
	"testing"
	"time"
)

func TestFingerprintIgnoresMutableClientDisplayName(t *testing.T) {
	event := SourceEvent{Timestamp: time.Date(2026, 8, 9, 1, 2, 3, 0, time.UTC), QueryName: "example.org", QueryType: "A", ClientIdentifier: "192.0.2.1", ResponseStatus: StatusAllowed}
	first := event.Fingerprint()
	event.ClientDisplayName = "Renamed client"
	if first != event.Fingerprint() {
		t.Fatal("client display-name enrichment changed source event identity")
	}
}

func TestNormalizeBoundsAndRejectsUnsafeElapsedValues(t *testing.T) {
	event := SourceEvent{Timestamp: time.Now(), QueryName: "Example.ORG.", QueryType: "a", ResponseStatus: "unknown", ElapsedMS: -1}
	if event.Normalize() {
		t.Fatal("negative processing time was accepted")
	}
	event.ElapsedMS = 1
	if !event.Normalize() || event.QueryName != "example.org" || event.QueryType != "A" || event.ResponseStatus != StatusOther {
		t.Fatalf("unexpected normalization: %+v", event)
	}
}

func TestNormalizePreservesDNSRootQuestion(t *testing.T) {
	event := SourceEvent{Timestamp: time.Now(), QueryName: ".", QueryType: "ns", ResponseStatus: StatusAllowed}
	if !event.Normalize() || event.QueryName != "." || event.QueryType != "NS" {
		t.Fatalf("root question was not preserved: %+v", event)
	}
}

func TestSupportsVersionBoundaries(t *testing.T) {
	for _, test := range []struct {
		version                 string
		supported, belowMinimum bool
	}{
		{"v0.106.3", false, true},
		{"v0.107.77", false, true},
		{"v0.107.78-rc.1", false, true},
		{"v0.107.78", true, false},
		{"v0.107.79", true, false},
		{"v0.107.80", true, false},
		{"v0.108.0-b.90", true, false},
		{"v0.108.0-b.91", true, false},
		{"v0.108.0", true, false},
		{"v1.0.0-b.1", true, false},
		{"v1.0.0", true, false},
		{"v1.8.0", true, false},
		{"v2.0.0", false, false},
		{"garbage", false, false},
		{"", false, false},
	} {
		t.Run(test.version, func(t *testing.T) {
			if got := SupportsVersion(test.version); got != test.supported {
				t.Errorf("SupportsVersion(%q) = %v, want %v", test.version, got, test.supported)
			}
			if got := VersionBelowMinimum(test.version); got != test.belowMinimum {
				t.Errorf("VersionBelowMinimum(%q) = %v, want %v", test.version, got, test.belowMinimum)
			}
		})
	}
}
