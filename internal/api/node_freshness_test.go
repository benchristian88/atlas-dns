package api

import (
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

func TestNodeFreshnessUsesLiveRuntimeHealthInterval(t *testing.T) {
	runtime := systemsettings.NewRuntimeStore(systemsettings.RuntimeSettings{NodeHealthInterval: 6 * time.Minute})
	server := &Server{healthInterval: 30 * time.Second}
	server.SetRuntimeSettings(runtime)

	if got := server.nodeStaleAfterSeconds(); got != 1080 {
		t.Fatalf("six-minute stale threshold=%d, want 1080", got)
	}
	runtime.Update(systemsettings.RuntimeSettings{NodeHealthInterval: 30 * time.Second})
	if got := server.nodeStaleAfterSeconds(); got != 90 {
		t.Fatalf("live default stale threshold=%d, want 90", got)
	}
}
