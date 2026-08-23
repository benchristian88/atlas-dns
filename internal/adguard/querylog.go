package adguard

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/querylog"
)

type queryLogResponse struct {
	Oldest string            `json:"oldest"`
	Data   []json.RawMessage `json:"data"`
}

type queryLogItem struct {
	Answer []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
		TTL   int64  `json:"ttl"`
	} `json:"answer"`
	Cached       bool   `json:"cached"`
	Upstream     string `json:"upstream"`
	AnswerDNSSEC bool   `json:"answer_dnssec"`
	Client       string `json:"client"`
	ClientID     string `json:"client_id"`
	ClientInfo   *struct {
		Name string `json:"name"`
	} `json:"client_info"`
	ClientProtocol string          `json:"client_proto"`
	ElapsedMS      json.RawMessage `json:"elapsedMs"`
	Question       struct {
		Name string `json:"name"`
		Host string `json:"host"`
		Type string `json:"type"`
	} `json:"question"`
	FilterID int64  `json:"filterId"`
	Rule     string `json:"rule"`
	Rules    []struct {
		Text         string `json:"text"`
		FilterListID int64  `json:"filter_list_id"`
	} `json:"rules"`
	Reason      string `json:"reason"`
	ServiceName string `json:"service_name"`
	Status      string `json:"status"`
	Time        string `json:"time"`
}

// SupportsQueryLog reports whether the version belongs to the supported
// AdGuard Home API generation.  Endpoint and response validation remain the
// decisive capability checks, including for newer, provisionally compatible
// patches in that generation.
func SupportsQueryLog(version string) bool { return querylog.SupportsVersion(version) }

func (r *ConfigurationReader) ReadQueryLogConfig(ctx context.Context, request domain.NodeProbeRequest, version string) (querylog.SourceConfig, error) {
	path := "/control/querylog_info"
	if supportsConfigurationPatch(version, 72) {
		path = "/control/querylog/config"
	}
	// The legacy querylog_info interval is expressed in days and can be a
	// fractional JSON number. Collection only needs these two privacy controls,
	// so deliberately avoid decoding unrelated version-variable fields.
	var response struct {
		Enabled           bool `json:"enabled"`
		AnonymizeClientIP bool `json:"anonymize_client_ip"`
	}
	if err := r.getOperationalResource(ctx, request, path, nil, &response); err != nil {
		return querylog.SourceConfig{}, err
	}
	return querylog.SourceConfig{Enabled: response.Enabled, AnonymizeClientIP: response.AnonymizeClientIP}, nil
}

func (r *ConfigurationReader) ReadQueryLog(ctx context.Context, request domain.NodeProbeRequest, olderThan string, limit int) (querylog.SourcePage, error) {
	if limit < 1 || limit > 500 {
		return querylog.SourcePage{}, domain.Validation("queryLogPage", "limit is outside supported bounds")
	}
	query := url.Values{
		"limit":           []string{strconv.Itoa(limit)},
		"search":          []string{""},
		"response_status": []string{"all"},
	}
	if olderThan = strings.TrimSpace(olderThan); olderThan != "" {
		if _, err := time.Parse(time.RFC3339Nano, olderThan); err != nil {
			return querylog.SourcePage{}, domain.Validation("olderThan", "must be an RFC3339 timestamp")
		}
		query.Set("older_than", olderThan)
	}
	var response queryLogResponse
	if err := r.getOperationalResource(ctx, request, "/control/querylog", query, &response); err != nil {
		return querylog.SourcePage{}, err
	}
	if len(response.Data) > limit {
		return querylog.SourcePage{}, domain.NewError(domain.ErrorNodeResponse, "the node query-log response exceeded the requested limit")
	}
	oldest := strings.TrimSpace(response.Oldest)
	if oldest != "" {
		if _, err := time.Parse(time.RFC3339Nano, oldest); err != nil {
			return querylog.SourcePage{}, domain.NewError(domain.ErrorNodeResponse, "the node query-log response used an invalid cursor")
		}
	}
	page := querylog.SourcePage{Oldest: oldest, Events: make([]querylog.SourceEvent, 0, len(response.Data))}
	for _, raw := range response.Data {
		var item queryLogItem
		if len(raw) > 64*1024 || json.Unmarshal(raw, &item) != nil {
			page.InvalidRecords++
			continue
		}
		event, ok := normalizeQueryLogItem(item)
		if !ok {
			page.InvalidRecords++
			continue
		}
		page.Events = append(page.Events, event)
	}
	return page, nil
}

func normalizeQueryLogItem(item queryLogItem) (querylog.SourceEvent, bool) {
	timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(item.Time))
	if err != nil {
		return querylog.SourceEvent{}, false
	}
	elapsed, ok := parseElapsedMS(item.ElapsedMS)
	if !ok {
		return querylog.SourceEvent{}, false
	}
	name := item.Question.Name
	if name == "" {
		name = item.Question.Host
	}
	event := querylog.SourceEvent{
		Timestamp: timestamp, QueryName: name, QueryType: item.Question.Type,
		ClientIdentifier: item.Client, ClientProtocol: item.ClientProtocol,
		ResponseStatus: mapFilteringStatus(item.Reason), ResponseCode: item.Status,
		ElapsedMS: elapsed, Upstream: item.Upstream, FilteringReason: item.Reason,
		ServiceName: item.ServiceName, Cached: item.Cached, AnswerDNSSEC: item.AnswerDNSSEC,
		Rules: make([]querylog.Rule, 0, len(item.Rules)), Answers: make([]querylog.Answer, 0, len(item.Answer)),
	}
	if event.ClientIdentifier == "" {
		event.ClientIdentifier = item.ClientID
	}
	if item.ClientInfo != nil {
		event.ClientDisplayName = item.ClientInfo.Name
	}
	for _, rule := range item.Rules {
		event.Rules = append(event.Rules, querylog.Rule{Text: rule.Text, FilterListID: rule.FilterListID})
	}
	if len(event.Rules) == 0 && item.Rule != "" {
		event.Rules = append(event.Rules, querylog.Rule{Text: item.Rule, FilterListID: item.FilterID})
	}
	for _, answer := range item.Answer {
		event.Answers = append(event.Answers, querylog.Answer{Type: answer.Type, Value: answer.Value, TTL: answer.TTL})
	}
	return event, event.Normalize()
}

func parseElapsedMS(raw json.RawMessage) (float64, bool) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, true
	}
	var value float64
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false
		}
		value = parsed
	} else if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	return value, value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func mapFilteringStatus(reason string) string {
	switch reason {
	case "NotFilteredNotFound", "NotFilteredWhiteList", "":
		return querylog.StatusAllowed
	case "FilteredBlackList", "FilteredBlockedService":
		return querylog.StatusBlocked
	case "FilteredSafeBrowsing":
		return querylog.StatusSafeBrowsing
	case "FilteredParental":
		return querylog.StatusParental
	case "FilteredSafeSearch":
		return querylog.StatusSafeSearch
	case "Rewrite", "RewriteEtcHosts", "RewriteRule":
		return querylog.StatusRewritten
	case "NotFilteredError", "FilteredInvalid":
		return querylog.StatusError
	default:
		return querylog.StatusOther
	}
}
