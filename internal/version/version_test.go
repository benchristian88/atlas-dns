package version

import (
	"os"
	"testing"
)

func TestBuildVersionMetadata(t *testing.T) {
	want := os.Getenv("WANT_BUILD_VERSION")
	if want == "" {
		want = "1.1.0-dev"
	}
	if Version != want {
		t.Fatalf("Version = %q, want %q", Version, want)
	}
}

func TestCurrentDistinguishesStableAndDevelopmentBuilds(t *testing.T) {
	originalVersion, originalCommit, originalBuiltAt := Version, Commit, BuiltAt
	t.Cleanup(func() { Version, Commit, BuiltAt = originalVersion, originalCommit, originalBuiltAt })
	Commit, BuiltAt = "abc123", "2026-08-09T00:00:00Z"
	Version = "0.9.0"
	if Current().Development {
		t.Fatal("stable build reported as development")
	}
	Version = "0.9.0-dev"
	if !Current().Development {
		t.Fatal("development suffix reported as stable")
	}
	Version = "0.9.0-rc.1"
	if !Current().Development {
		t.Fatal("prerelease reported as stable")
	}
}
