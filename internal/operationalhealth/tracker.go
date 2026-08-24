package operationalhealth

import (
	"sort"
	"sync"
	"time"
)

type workerState struct {
	Worker
	startedAt *time.Time
}

// Tracker keeps bounded process-local worker state. Durable collector evidence
// remains in PostgreSQL; after restart a worker is deliberately unknown until
// its first run rather than presenting stale process state as current.
type Tracker struct {
	mu      sync.RWMutex
	workers map[string]workerState
	now     func() time.Time
}

func NewTracker() *Tracker {
	return &Tracker{workers: map[string]workerState{}, now: time.Now}
}

func (t *Tracker) Register(name string, paused bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := Unknown
	if paused {
		state = Paused
	}
	t.workers[name] = workerState{Worker: Worker{Name: name, State: state}}
}

func (t *Tracker) Pause(name string, next time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	item := t.workers[name]
	item.Name, item.State, item.Running, item.startedAt = name, Paused, false, nil
	item.NextScheduledAt = timePtr(next)
	item.ErrorCode, item.ConsecutiveFailures = "", 0
	t.workers[name] = item
}

func (t *Tracker) Start(name string, next time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now().UTC()
	item := t.workers[name]
	item.Name, item.State, item.Running = name, Healthy, true
	item.LastAttemptAt, item.NextScheduledAt, item.startedAt = &now, timePtr(next), &now
	item.RunsTotal++
	t.workers[name] = item
}

func (t *Tracker) Success(name string, next time.Time) {
	t.finish(name, "", next)
}

func (t *Tracker) Failure(name, code string, next time.Time) {
	t.finish(name, code, next)
}

func (t *Tracker) finish(name, code string, next time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now().UTC()
	item := t.workers[name]
	item.Name, item.Running, item.startedAt = name, false, nil
	item.NextScheduledAt = timePtr(next)
	if code == "" {
		item.State, item.LastSuccessAt, item.ConsecutiveFailures, item.ErrorCode = Healthy, &now, 0, ""
	} else {
		item.ConsecutiveFailures++
		item.FailuresTotal++
		item.State, item.LastFailureAt, item.ErrorCode = Degraded, &now, code
		if item.ConsecutiveFailures >= 3 {
			item.State = Failed
		}
	}
	t.workers[name] = item
}

func (t *Tracker) Snapshot() []Worker {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now().UTC()
	items := make([]Worker, 0, len(t.workers))
	for _, state := range t.workers {
		item := state.Worker
		if state.startedAt != nil {
			item.CurrentDurationMS = now.Sub(*state.startedAt).Milliseconds()
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}
