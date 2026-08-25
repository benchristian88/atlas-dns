package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/systemsettings"
)

type operationalHistoryStoreFake struct {
	before chan time.Time
}

func (s *operationalHistoryStoreFake) PruneOperationalHistory(_ context.Context, before time.Time, limit int) (int64, error) {
	if limit != 10000 {
		panic("unexpected cleanup batch")
	}
	s.before <- before
	return 2, nil
}

func TestOperationalHistoryRetentionRunsImmediatelyWithPersistedSetting(t *testing.T) {
	store := &operationalHistoryStoreFake{before: make(chan time.Time, 1)}
	settings := systemsettings.NewRuntimeStore(systemsettings.RuntimeSettings{OperationalHistoryRetention: 14 * 24 * time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunOperationalHistoryRetention(ctx, store, settings, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	select {
	case before := <-store.before:
		age := time.Since(before)
		if age < 14*24*time.Hour-time.Minute || age > 14*24*time.Hour+time.Minute {
			t.Fatalf("cleanup cutoff age = %v", age)
		}
	case <-time.After(time.Second):
		t.Fatal("retention did not run at startup")
	}
}
