package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type clusterCollectorFake struct {
	mu       sync.Mutex
	clusters []string
	err      error
}

func (f *clusterCollectorFake) PollClusterNow(_ context.Context, clusterID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clusters = append(f.clusters, clusterID)
	return f.err
}

type dnsClusterCollectorFake struct {
	mu       sync.Mutex
	clusters []string
	err      error
}

func (f *dnsClusterCollectorFake) PollCluster(_ context.Context, clusterID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clusters = append(f.clusters, clusterID)
	return f.err
}

func TestOnboardingCollectorRunsEveryInitialPassForTheCluster(t *testing.T) {
	health := &clusterCollectorFake{}
	statistics := &clusterCollectorFake{err: errors.New("statistics unavailable")}
	queryLog := &clusterCollectorFake{}
	dns := &dnsClusterCollectorFake{}
	collector := NewOnboardingCollector(health, statistics, queryLog, dns)

	err := collector.CollectInitial(context.Background(), "cluster-one")
	if err == nil || !strings.Contains(err.Error(), "statistics initial collection") {
		t.Fatalf("CollectInitial() error = %v", err)
	}
	for name, clusters := range map[string][]string{
		"health": health.clusters, "statistics": statistics.clusters,
		"query log": queryLog.clusters, "DNS": dns.clusters,
	} {
		if len(clusters) != 1 || clusters[0] != "cluster-one" {
			t.Fatalf("%s clusters = %#v", name, clusters)
		}
	}
}
