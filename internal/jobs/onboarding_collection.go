package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type clusterCollector interface {
	PollClusterNow(context.Context, string) error
}

type dnsClusterCollector interface {
	PollCluster(context.Context, string) error
}

// OnboardingCollector primes the durable dashboard evidence after guided
// setup. Each collector remains independent so one unavailable node or data
// source cannot prevent the other initial passes from running.
type OnboardingCollector struct {
	health     clusterCollector
	statistics clusterCollector
	queryLog   clusterCollector
	dns        dnsClusterCollector
}

func NewOnboardingCollector(health, statistics, queryLog clusterCollector, dns dnsClusterCollector) *OnboardingCollector {
	return &OnboardingCollector{health: health, statistics: statistics, queryLog: queryLog, dns: dns}
}

func (c *OnboardingCollector) CollectInitial(ctx context.Context, clusterID string) error {
	collections := []struct {
		name string
		run  func(context.Context, string) error
	}{
		{name: "node health", run: c.health.PollClusterNow},
		{name: "DNS health", run: c.dns.PollCluster},
		{name: "statistics", run: c.statistics.PollClusterNow},
		{name: "query log", run: c.queryLog.PollClusterNow},
	}
	var group sync.WaitGroup
	errorsChannel := make(chan error, len(collections))
	for _, collection := range collections {
		collection := collection
		group.Add(1)
		go func() {
			defer group.Done()
			if err := collection.run(ctx, clusterID); err != nil {
				errorsChannel <- fmt.Errorf("%s initial collection: %w", collection.name, err)
			}
		}()
	}
	group.Wait()
	close(errorsChannel)
	var collectionErrors []error
	for err := range errorsChannel {
		collectionErrors = append(collectionErrors, err)
	}
	return errors.Join(collectionErrors...)
}
