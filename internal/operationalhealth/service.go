package operationalhealth

import (
	"context"
	"fmt"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/haoperations"
	"github.com/benchristian88/atlas-dns/internal/inventory"
	"github.com/benchristian88/atlas-dns/internal/querylog"
	"github.com/benchristian88/atlas-dns/internal/systemsettings"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
)

type Repository interface {
	ClusterByID(context.Context, string) (domain.Cluster, error)
	ListNodes(context.Context, string) ([]domain.Node, error)
	LatestSnapshots(context.Context, string) ([]inventory.Snapshot, error)
	LatestSuccessfulSnapshots(context.Context, string) ([]inventory.Snapshot, error)
	CapabilityProfiles(context.Context, string) ([]inventory.CapabilityProfile, error)
	LatestStatisticsAttempts(context.Context, string, string) ([]telemetry.NodeAttempt, error)
	QueryLogCheckpoints(context.Context, string, string) ([]querylog.Checkpoint, error)
	OperationalDatabase(context.Context, time.Duration, time.Duration) (Database, error)
}

type HAReader interface {
	Summary(context.Context, string) (haoperations.HASummary, error)
}

type Options struct {
	NodeInterval        time.Duration
	RequestTimeout      time.Duration
	StatisticsInterval  time.Duration
	QueryLogInterval    time.Duration
	StatisticsRetention time.Duration
	QueryLogRetention   time.Duration
	QueryLogEnabled     bool
}

type Service struct {
	repository Repository
	tracker    *Tracker
	options    Options
	now        func() time.Time
	ha         HAReader
	settings   interface {
		RuntimeSettings() systemsettings.RuntimeSettings
	}
}

func (s *Service) SetHAOperations(reader HAReader) { s.ha = reader }
func (s *Service) SetRuntimeSettings(provider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}) {
	s.settings = provider
}

func NewService(repository Repository, tracker *Tracker, options Options) *Service {
	return &Service{repository: repository, tracker: tracker, options: options, now: time.Now}
}

func (s *Service) Status(ctx context.Context, clusterID string) (Status, error) {
	options := s.options
	if s.settings != nil {
		current := s.settings.RuntimeSettings()
		options.NodeInterval = current.NodeHealthInterval
		options.StatisticsInterval = current.StatisticsPollInterval
		options.QueryLogInterval = current.QueryLogPollInterval
		options.QueryLogRetention = current.QueryLogRetention
		options.QueryLogEnabled = current.QueryLogCollection
	}
	if !domain.ValidID(clusterID) {
		return Status{}, domain.Validation("clusterId", "must be a valid UUID")
	}
	if _, err := s.repository.ClusterByID(ctx, clusterID); err != nil {
		return Status{}, err
	}
	nodes, err := s.repository.ListNodes(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	if len(nodes) > 1000 {
		return Status{}, domain.NewError(domain.ErrorInternal, "operational status is unavailable for an oversized cluster")
	}
	snapshots, err := s.repository.LatestSnapshots(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	successfulSnapshots, err := s.repository.LatestSuccessfulSnapshots(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	profiles, err := s.repository.CapabilityProfiles(ctx, clusterID)
	if err != nil {
		return Status{}, err
	}
	attempts, err := s.repository.LatestStatisticsAttempts(ctx, clusterID, "")
	if err != nil {
		return Status{}, err
	}
	checkpoints, err := s.repository.QueryLogCheckpoints(ctx, clusterID, "")
	if err != nil {
		return Status{}, err
	}
	dnsProbes := []haoperations.DNSProbeResult{}
	if repository, ok := s.repository.(interface {
		LatestDNSProbes(context.Context, string) ([]haoperations.DNSProbeResult, error)
	}); ok {
		dnsProbes, err = repository.LatestDNSProbes(ctx, clusterID)
		if err != nil {
			return Status{}, err
		}
	}
	database, dbErr := s.repository.OperationalDatabase(ctx, options.StatisticsRetention, options.QueryLogRetention)
	if dbErr != nil {
		database.State, database.ErrorCode = Failed, "DATABASE_UNAVAILABLE"
	}

	now := s.now().UTC()
	status := Status{GeneratedAt: now, ClusterID: clusterID, API: Healthy, Database: database, Workers: s.tracker.Snapshot()}
	status.Nodes = nodeHealth(nodes, now, maxDuration(3*options.NodeInterval, time.Minute))
	status.DNSService = dnsServiceHealth(nodes, dnsProbes, now, haoperations.DNSFreshnessWindow(options.NodeInterval), options.NodeInterval)
	if s.ha != nil {
		status.HA, _ = s.ha.Summary(ctx, clusterID)
	}
	status.Observation = observationHealth(nodes, snapshots, successfulSnapshots, profiles, now, maxDuration(3*options.NodeInterval+options.RequestTimeout, 2*time.Minute), options.NodeInterval)
	status.Statistics = statisticsHealth(nodes, attempts, now, maxDuration(2*options.StatisticsInterval+options.RequestTimeout, 3*time.Hour), options.StatisticsInterval)
	status.QueryLog = queryLogHealth(nodes, checkpoints, now, maxDuration(3*options.QueryLogInterval, 2*time.Minute), options.QueryLogInterval, options.QueryLogEnabled)
	status.Summary = aggregate(status)
	return status, nil
}

func dnsServiceHealth(nodes []domain.Node, probes []haoperations.DNSProbeResult, now time.Time, staleAfter, interval time.Duration) CollectionSummary {
	byNode := map[string]haoperations.DNSProbeResult{}
	for _, probe := range probes {
		byNode[probe.NodeID] = probe
	}
	result := CollectionSummary{Nodes: []NodeSubsystem{}}
	for _, node := range nodes {
		item := NodeSubsystem{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			item.State = Paused
		} else {
			result.ExpectedNodes++
			item.counted = true
			probe, ok := byNode[node.ID]
			switch {
			case node.MaintenanceMode:
				item.State = Maintenance
			case !ok:
				item.State = Unknown
			default:
				item.LastAttemptAt = &probe.ProbedAt
				item.NextScheduledAt = timePointer(probe.ProbedAt.Add(interval))
				item.ErrorCode = probe.ErrorCode
				if probe.Status == "healthy" {
					item.State = Healthy
					item.LastSuccessAt = &probe.ProbedAt
					if now.Sub(probe.ProbedAt) > staleAfter {
						item.State = Stale
					}
				} else {
					item.State = Failed
				}
				if probe.LatencyMS != nil {
					lag := int64(*probe.LatencyMS)
					item.LagSeconds = &lag
				}
			}
		}
		result.Nodes = append(result.Nodes, item)
	}
	finishCollection(&result)
	return result
}

func nodeHealth(nodes []domain.Node, now time.Time, staleAfter time.Duration) []NodeSubsystem {
	result := make([]NodeSubsystem, 0, len(nodes))
	for _, node := range nodes {
		item := NodeSubsystem{NodeID: node.ID, NodeName: node.Name, LastAttemptAt: node.LastPolledAt, LastSuccessAt: node.LastSeenAt, ErrorCode: node.LastErrorCode}
		switch {
		case !node.Enabled:
			item.State = Paused
		case node.MaintenanceMode:
			item.State = Maintenance
		case node.HealthStatus == domain.NodeUnreachable:
			item.State = Failed
		case node.HealthStatus == domain.NodeIncompatible:
			item.State = Unsupported
		case node.LastPolledAt == nil:
			item.State = Unknown
		case now.Sub(*node.LastPolledAt) > staleAfter:
			item.State = Stale
		default:
			item.State = Healthy
		}
		if node.LastSeenAt != nil {
			lag := int64(now.Sub(*node.LastSeenAt).Seconds())
			item.LagSeconds = &lag
		}
		result = append(result, item)
	}
	return result
}

func observationHealth(nodes []domain.Node, snapshots, successful []inventory.Snapshot, profiles []inventory.CapabilityProfile, now time.Time, staleAfter, interval time.Duration) CollectionSummary {
	byNode := map[string]inventory.Snapshot{}
	for _, value := range snapshots {
		byNode[value.NodeID] = value
	}
	lastSuccess := map[string]inventory.Snapshot{}
	for _, value := range successful {
		lastSuccess[value.NodeID] = value
	}
	capabilities := map[string]inventory.CapabilityProfile{}
	for _, value := range profiles {
		capabilities[value.NodeID] = value
	}
	result := CollectionSummary{Nodes: []NodeSubsystem{}}
	for _, node := range nodes {
		item := NodeSubsystem{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			item.State = Paused
		} else {
			result.ExpectedNodes++
			item.counted = true
			if profile, exists := capabilities[node.ID]; exists {
				item.CapabilityRefreshedAt = &profile.RefreshedAt
				switch {
				case profile.Compatibility == string(domain.CompatibilityUnsupported):
					item.CapabilityState = Unsupported
				case now.Sub(profile.RefreshedAt) > staleAfter:
					item.CapabilityState = Stale
				default:
					item.CapabilityState = Healthy
				}
			} else {
				item.CapabilityState = Unknown
			}
			snap, ok := byNode[node.ID]
			switch {
			case node.MaintenanceMode:
				item.State = Maintenance
			case !ok:
				item.State = Unknown
			default:
				item.LastAttemptAt = &snap.ObservedAt
				item.NextScheduledAt = timePointer(snap.ObservedAt.Add(interval))
				item.ErrorCode = snap.ErrorCode
				if success, exists := lastSuccess[node.ID]; exists {
					item.LastSuccessAt = &success.ObservedAt
				}
				if snap.CollectionStatus == "succeeded" {
					item.State = Healthy
				} else {
					item.State = Failed
					item.ConsecutiveFailures = 1
				}
				if now.Sub(snap.ObservedAt) > staleAfter {
					item.State = Stale
				}
			}
		}
		result.Nodes = append(result.Nodes, item)
	}
	finishCollection(&result)
	return result
}

func statisticsHealth(nodes []domain.Node, attempts []telemetry.NodeAttempt, now time.Time, staleAfter, interval time.Duration) CollectionSummary {
	byNode := map[string]telemetry.NodeAttempt{}
	for _, value := range attempts {
		byNode[value.NodeID] = value
	}
	result := CollectionSummary{Nodes: []NodeSubsystem{}}
	for _, node := range nodes {
		item := NodeSubsystem{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			item.State = Paused
		} else {
			result.ExpectedNodes++
			item.counted = true
			attempt, ok := byNode[node.ID]
			switch {
			case node.MaintenanceMode:
				item.State = Maintenance
			case !ok:
				item.State = Unknown
			default:
				item.LastAttemptAt = &attempt.CompletedAt
				item.NextScheduledAt = timePointer(attempt.CompletedAt.Add(interval))
				item.ErrorCode = attempt.ErrorCode
				item.RecordsReceived = attempt.CollectedRanges
				item.ConsecutiveFailures = attempt.ConsecutiveFailures
				item.LastSuccessAt = attempt.LastSuccessAt
				if attempt.Status == "unsupported" {
					item.State = Unsupported
				} else if attempt.Status == "maintenance" {
					// A maintenance attempt is an intentional skipped poll, not a
					// collection failure.  Once the node has returned to service,
					// continue using a still-fresh prior success until the next
					// scheduled poll replaces the maintenance evidence.
					if attempt.LastSuccessAt == nil {
						item.State = Unknown
					} else if now.Sub(*attempt.LastSuccessAt) > staleAfter {
						item.State = Stale
					} else {
						item.State = Healthy
						item.ErrorCode = ""
					}
				} else if attempt.Status == "succeeded" {
					item.State = Healthy
				} else if attempt.Status == "partial" {
					item.State = Degraded
				} else {
					item.State = Failed
				}
				if item.State != Unsupported && item.State != Maintenance && now.Sub(attempt.CompletedAt) > staleAfter {
					item.State = Stale
				}
			}
		}
		result.Nodes = append(result.Nodes, item)
	}
	finishCollection(&result)
	return result
}

func queryLogHealth(nodes []domain.Node, checkpoints []querylog.Checkpoint, now time.Time, staleAfter, interval time.Duration, enabled bool) CollectionSummary {
	byNode := map[string]querylog.Checkpoint{}
	for _, value := range checkpoints {
		byNode[value.NodeID] = value
	}
	result := CollectionSummary{Nodes: []NodeSubsystem{}}
	for _, node := range nodes {
		item := NodeSubsystem{NodeID: node.ID, NodeName: node.Name}
		if !node.Enabled {
			item.State = Paused
		} else {
			result.ExpectedNodes++
			item.counted = true
			checkpoint, ok := byNode[node.ID]
			switch {
			case !enabled:
				item.State, item.ErrorCode = Paused, "QUERY_LOG_COLLECTION_DISABLED"
			case node.MaintenanceMode:
				item.State = Maintenance
			case !ok:
				item.State = Unknown
			default:
				item.LastAttemptAt, item.LastSuccessAt = &checkpoint.LastAttemptAt, checkpoint.LastSuccessAt
				item.NextScheduledAt = timePointer(checkpoint.LastAttemptAt.Add(interval))
				item.ErrorCode = checkpoint.ErrorCode
				item.GapDetected, item.GapReason = checkpoint.GapDetected, checkpoint.GapReason
				if checkpoint.LastStatus == "unsupported" {
					item.State = Unsupported
				} else if checkpoint.LastStatus == "maintenance" {
					item.State = Maintenance
				} else if checkpoint.LastStatus == "logging_disabled" {
					item.State = Paused
				} else if checkpoint.LastStatus == "failed" {
					item.State = Failed
					item.ConsecutiveFailures = checkpoint.ConsecutiveFailures
				} else {
					item.State = Healthy
				}
				if checkpoint.SourceNewestAt != nil {
					lag := int64(now.Sub(*checkpoint.SourceNewestAt).Seconds())
					item.LagSeconds = &lag
				}
				if checkpoint.GapDetected && item.State == Healthy {
					item.State = Degraded
				}
				if item.State != Unsupported && item.State != Maintenance && item.State != Paused && now.Sub(checkpoint.LastAttemptAt) > staleAfter {
					item.State = Stale
				}
			}
		}
		result.Nodes = append(result.Nodes, item)
	}
	finishCollection(&result)
	return result
}

func finishCollection(result *CollectionSummary) {
	paused, maintenance, unknown, failed, degraded := 0, 0, 0, 0, 0
	for _, item := range result.Nodes {
		if !item.counted {
			continue
		}
		switch item.State {
		case Healthy:
			result.CurrentNodes++
		case Degraded:
			result.CurrentNodes++
			degraded++
		case Stale:
			result.StaleNodes++
		case Unsupported:
			result.UnsupportedNodes++
		case Paused:
			paused++
		case Maintenance:
			maintenance++
		case Unknown:
			unknown++
		case Failed:
			failed++
		}
	}
	if result.ExpectedNodes > 0 {
		result.CoveragePercent = float64(result.CurrentNodes) * 100 / float64(result.ExpectedNodes)
	}
	result.State = Healthy
	considered := result.ExpectedNodes
	if considered == 0 {
		result.State = Unknown
	} else if paused == considered {
		result.State = Paused
	} else if maintenance == considered {
		result.State = Maintenance
	} else if result.UnsupportedNodes == considered {
		result.State = Unsupported
	} else if unknown == considered {
		result.State = Unknown
	} else if failed == considered {
		result.State = Failed
	} else if degraded > 0 {
		result.State = Degraded
	} else if result.CurrentNodes == 0 {
		result.State = Degraded
	} else if result.CurrentNodes < result.ExpectedNodes {
		result.State = Degraded
	}
}

func aggregate(status Status) Summary {
	summary := Summary{State: Healthy, Message: "All monitored controller subsystems are healthy."}
	for _, node := range status.Nodes {
		if node.State != Paused {
			summary.ExpectedNodes++
		}
		if node.State == Healthy {
			summary.HealthyNodes++
		}
		if node.State == Failed || node.State == Stale {
			summary.State, summary.ActionRequired = Degraded, true
		}
	}
	for _, state := range []State{status.DNSService.State, status.Observation.State, status.Statistics.State, status.QueryLog.State} {
		if state == Degraded || state == Failed || state == Stale {
			summary.State, summary.ActionRequired = Degraded, true
		}
	}
	for _, worker := range status.Workers {
		if worker.State == Failed {
			summary.State, summary.ActionRequired = Degraded, true
		}
	}
	if status.Database.State == Failed {
		summary.State, summary.ActionRequired, summary.Message = Failed, true, "PostgreSQL is unavailable; normal controller requests cannot be served."
	} else if summary.State == Degraded {
		summary.Message = "One or more integrations are stale or degraded; DNS service on the nodes is unaffected."
	}
	return summary
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
func timePointer(value time.Time) *time.Time { value = value.UTC(); return &value }

func (s Status) ValidateBounded() error {
	if len(s.Nodes) > 1000 || len(s.DNSService.Nodes) > 1000 || len(s.Observation.Nodes) > 1000 || len(s.Statistics.Nodes) > 1000 || len(s.QueryLog.Nodes) > 1000 || len(s.Workers) > 100 {
		return fmt.Errorf("operational status node payload is not bounded")
	}
	return nil
}
