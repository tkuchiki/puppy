package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

type DepRow struct {
	OutgoingResource string  `json:"outgoing_resource"` // 送信側（client span）の resource_name
	PeerService      string  `json:"peer_service"`      // @peer.service（DB/外部API 等）
	Count            float64 `json:"count"`
}

type AccurateResult struct {
	Mode              string    `json:"mode"` // "accurate"
	Site              string    `json:"site"`
	Service           string    `json:"service"`
	Env               string    `json:"env"`
	IncomingEndpoint  string    `json:"incoming_endpoint"` // 対象の受信エンドポイント（resource_name）
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	CollectedTraceIDs int       `json:"collected_trace_ids"`
	ExternalDeps      []DepRow  `json:"external_deps"` // outgoing_resource × @peer.service
	InternalServices  []Bucket  `json:"internal_services"`
}

type FastResult struct {
	Mode         string    `json:"mode"` // "fast"
	Site         string    `json:"site"`
	Service      string    `json:"service"`
	Env          string    `json:"env"`
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	Approximate  bool      `json:"approximate"`
	ExternalDeps []DepRow  `json:"external_deps"`
}

type Bucket struct {
	Name  string  `json:"name"`
	Count float64 `json:"count"`
}

type ServiceCmd struct {
	Site      string        `help:"Datadog site" default:"datadoghq.com"`
	CacheDir  string        `help:"Cache directory" default:"./cache"`
	CacheTTL  time.Duration `help:"Cache TTL" default:"1h"`
	Mode      string        `help:"accurate or fast" default:"fast"`
	Service   string        `help:"Service name"`
	Env       string        `help:"Environment name"`
	Endpoint  string        `help:"Endpoint"`
	Loopback  time.Duration `help:"Loopback interval" default:"1h"`
	From      time.Time     `help:"From"`
	To        time.Time     `help:"To"`
	PageLimit int           `help:"Page limit"`
	MaxTraces int           `help:"Maximum number of traces"`
}

func (s *ServiceCmd) Run(cli *CLI) error {
	spanClient, err := NewSpanClient(s.CacheDir, s.CacheTTL, s.Site, cli.apiClient)
	if err != nil {
		return err
	}

	if s.From.IsZero() || s.To.IsZero() {
		s.From = time.Now().Add(-s.Loopback)
		s.To = time.Now()
	}

	ctx := newContext(cli.ApiKey, cli.AppKey)

	switch s.Mode {
	case "fast":
		res, err := spanClient.EndpointDependenciesFast(ctx, s.Service, s.Env, s.From, s.To)
		if err != nil {
			return err
		}

		fmt.Println(res)
	case "accurate":
		res, err := spanClient.EndpointDependenciesAccurate(ctx, s.Service, s.Env, s.Endpoint, s.From, s.To, s.PageLimit, s.MaxTraces)
		if err != nil {
			return err
		}

		fmt.Println(res)
	}

	return nil
}

type SpanClient struct {
	spansApi *datadogV2.SpansApi
	cache    *fileCache
	cacheTTL time.Duration
	site     string
}

func NewSpanClient(cacheDir string, cacheTTL time.Duration, site string, apiClient *datadog.APIClient) (*SpanClient, error) {
	cache, err := newFileCache(cacheDir)
	if err != nil {
		return nil, err
	}

	return &SpanClient{
		cache:    cache,
		cacheTTL: cacheTTL,
		site:     site,
		spansApi: datadogV2.NewSpansApi(apiClient),
	}, nil
}

func (s *SpanClient) Close() error { return nil }

// レスポンスヘッダから軽い待機（保守的なスロットル）
func (s *SpanClient) throttleFromHeaders(r *http.Response) {
	if r == nil {
		return
	}
	h := r.Header
	if h.Get("X-RateLimit-Remaining") == "0" {
		time.Sleep(3 * time.Second)
	}
}

//
// ========= Public API（集計のエントリ） =========
//

// accurate: endpoint（受信）→ その処理中の外部依存（peer）＆内部依存（他サービス）
func (s *SpanClient) EndpointDependenciesAccurate(
	ctx context.Context,
	service, env, endpoint string,
	from, to time.Time, pageLimit, maxTraces int,
) (*AccurateResult, error) {

	traceIDs, err := s.listTraceIDs(ctx, service, env, endpoint, from, to, pageLimit, maxTraces)
	if err != nil {
		return nil, err
	}

	ext, err := s.aggregateClientPeersByTrace(ctx, service, traceIDs, from, to)
	if err != nil {
		return nil, err
	}
	intsvc, err := s.aggregateOtherServicesByTrace(ctx, service, traceIDs, from, to)
	if err != nil {
		return nil, err
	}

	return &AccurateResult{
		Mode:              "accurate",
		Site:              s.site,
		Service:           service,
		Env:               env,
		IncomingEndpoint:  endpoint,
		From:              from.UTC(),
		To:                to.UTC(),
		CollectedTraceIDs: len(traceIDs),
		ExternalDeps:      ext,
		InternalServices:  toSortedBuckets(intsvc),
	}, nil
}

// fast: 受信エンドポイント切り分け無しで、サービス全体の client span を一発集計
func (s *SpanClient) EndpointDependenciesFast(ctx context.Context, service, env string, from, to time.Time) (*FastResult, error) {
	q := fmt.Sprintf(`service:%q env:%q @span.kind:client`, service, env)
	body := datadogV2.SpansAggregateRequest{
		Data: &datadogV2.SpansAggregateData{
			Attributes: &datadogV2.SpansAggregateRequestAttributes{
				Compute: []datadogV2.SpansCompute{
					{Aggregation: datadogV2.SPANSAGGREGATIONFUNCTION_COUNT},
				},
				Filter: &datadogV2.SpansQueryFilter{
					From:  datadog.PtrString(from.UTC().Format(time.RFC3339)),
					To:    datadog.PtrString(to.UTC().Format(time.RFC3339)),
					Query: datadog.PtrString(q),
				},
				GroupBy: []datadogV2.SpansGroupBy{
					{Facet: "resource_name"},
					{Facet: "@peer.service"},
				},
			},
			Type: datadogV2.SPANSAGGREGATEREQUESTTYPE_AGGREGATE_REQUEST.Ptr(),
		},
	}

	// キャッシュ
	var raw map[string]any
	if ok, _ := s.cache.Get("v2.AggregateSpansFast", s.site, body, s.cacheTTL, &raw); !ok {
		resp, r, err := s.spansApi.AggregateSpans(ctx, body)
		s.throttleFromHeaders(r)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		_ = json.Unmarshal(b, &raw)
		s.cache.Set("v2.AggregateSpansFast", s.site, body, raw)
	}

	rows := parseAggregatePairs(raw, "resource_name", "@peer.service")
	return &FastResult{
		Mode:         "fast",
		Site:         s.site,
		Service:      service,
		Env:          env,
		From:         from.UTC(),
		To:           to.UTC(),
		Approximate:  true,
		ExternalDeps: rows,
	}, nil
}

//
// ========= 内部実装 =========
//

func (s *SpanClient) listTraceIDs(ctx context.Context, service, env, endpoint string, from, to time.Time, pageLimit, maxTraces int) ([]string, error) {
	q := fmt.Sprintf(`service:%q env:%q resource_name:%q @span.kind:server`, service, env, endpoint)

	req := datadogV2.SpansListRequest{
		Data: &datadogV2.SpansListRequestData{
			Attributes: &datadogV2.SpansListRequestAttributes{
				Filter: &datadogV2.SpansQueryFilter{
					From:  datadog.PtrString(from.UTC().Format(time.RFC3339)),
					To:    datadog.PtrString(to.UTC().Format(time.RFC3339)),
					Query: datadog.PtrString(q),
				},
				Options: &datadogV2.SpansQueryOptions{Timezone: datadog.PtrString("UTC")},
				Page:    &datadogV2.SpansListRequestPage{Limit: datadog.PtrInt32(int32(pageLimit))},
				Sort:    datadogV2.SPANSSORT_TIMESTAMP_ASCENDING.Ptr(),
			},
			Type: datadogV2.SPANSLISTREQUESTTYPE_SEARCH_REQUEST.Ptr(),
		},
	}

	var traceIDs []string
	cursor := ""

	for {
		if cursor != "" {
			req.Data.Attributes.Page.Cursor = datadog.PtrString(cursor)
		}

		var resp datadogV2.SpansListResponse
		if ok, _ := s.cache.Get("v2.ListSpans", s.site, req, s.cacheTTL, &resp); !ok {
			apiResp, r, err := s.spansApi.ListSpans(ctx, req)
			s.throttleFromHeaders(r)
			if err != nil {
				return nil, fmt.Errorf("ListSpans: %w", err)
			}
			resp = apiResp
			s.cache.Set("v2.ListSpans", s.site, req, resp)
		}

		for _, s := range resp.GetData() {
			attr := s.GetAttributes()
			if tid := attr.GetTraceId(); tid != "" {
				traceIDs = append(traceIDs, tid)
				if maxTraces > 0 && len(traceIDs) >= maxTraces {
					return uniq(traceIDs), nil
				}
			}
		}
		meta := resp.GetMeta()
		page := meta.GetPage()
		after := page.GetAfter()
		/*
			status := meta.GetStatus()
			if after == "" || strings.EqualFold(, "timeout") {
				break
			}
		*/

		cursor = after
	}
	return uniq(traceIDs), nil
}

// 外部依存（client span）: 自サービスに限定し、resource_name×@peer.service でカウント
func (s *SpanClient) aggregateClientPeersByTrace(ctx context.Context, service string, traceIDs []string, from, to time.Time) ([]DepRow, error) {
	if len(traceIDs) == 0 {
		return nil, nil
	}
	const chunk = 80
	var all []DepRow

	for i := 0; i < len(traceIDs); i += chunk {
		end := i + chunk
		if end > len(traceIDs) {
			end = len(traceIDs)
		}
		ids := strings.Join(traceIDs[i:end], " OR ")
		q := fmt.Sprintf(`trace_id:(%s) service:%q @span.kind:client`, ids, service)

		req := datadogV2.SpansAggregateRequest{
			Data: &datadogV2.SpansAggregateData{
				Attributes: &datadogV2.SpansAggregateRequestAttributes{
					Compute: []datadogV2.SpansCompute{{Aggregation: datadogV2.SPANSAGGREGATIONFUNCTION_COUNT}},
					Filter: &datadogV2.SpansQueryFilter{
						From:  datadog.PtrString(from.UTC().Format(time.RFC3339)),
						To:    datadog.PtrString(to.UTC().Format(time.RFC3339)),
						Query: datadog.PtrString(q),
					},
					GroupBy: []datadogV2.SpansGroupBy{
						{Facet: "resource_name"},
						{Facet: "@peer.service"},
					},
				},
				Type: datadogV2.SPANSAGGREGATEREQUESTTYPE_AGGREGATE_REQUEST.Ptr(),
			},
		}

		var raw map[string]any
		if ok, _ := s.cache.Get("v2.AggregateSpansClient", s.site, req, s.cacheTTL, &raw); !ok {
			resp, r, err := s.spansApi.AggregateSpans(ctx, req)
			s.throttleFromHeaders(r)
			if err != nil {
				return nil, fmt.Errorf("AggregateSpans(client): %w", err)
			}
			b, _ := json.Marshal(resp)
			_ = json.Unmarshal(b, &raw)
			s.cache.Set("v2.AggregateSpansClient", s.site, req, raw)
		}
		part := parseAggregatePairs(raw, "resource_name", "@peer.service")
		all = append(all, part...)
	}
	return reduceRows(all), nil
}

// 内部依存（他サービスの server span）: -service:<self> で service をグルーピング
func (s *SpanClient) aggregateOtherServicesByTrace(ctx context.Context, service string, traceIDs []string, from, to time.Time) (map[string]float64, error) {
	out := map[string]float64{}
	if len(traceIDs) == 0 {
		return out, nil
	}
	const chunk = 120

	for i := 0; i < len(traceIDs); i += chunk {
		end := i + chunk
		if end > len(traceIDs) {
			end = len(traceIDs)
		}
		ids := strings.Join(traceIDs[i:end], " OR ")
		q := fmt.Sprintf(`trace_id:(%s) @span.kind:server -service:%q`, ids, service)

		req := datadogV2.SpansAggregateRequest{
			Data: &datadogV2.SpansAggregateData{
				Attributes: &datadogV2.SpansAggregateRequestAttributes{
					Compute: []datadogV2.SpansCompute{{Aggregation: datadogV2.SPANSAGGREGATIONFUNCTION_COUNT}},
					Filter: &datadogV2.SpansQueryFilter{
						From:  datadog.PtrString(from.UTC().Format(time.RFC3339)),
						To:    datadog.PtrString(to.UTC().Format(time.RFC3339)),
						Query: datadog.PtrString(q),
					},
					GroupBy: []datadogV2.SpansGroupBy{
						{Facet: "service"},
					},
				},
				Type: datadogV2.SPANSAGGREGATEREQUESTTYPE_AGGREGATE_REQUEST.Ptr(),
			},
		}

		var raw map[string]any
		if ok, _ := s.cache.Get("v2.AggregateSpansInternal", s.site, req, s.cacheTTL, &raw); !ok {
			resp, r, err := s.spansApi.AggregateSpans(ctx, req)
			s.throttleFromHeaders(r)
			if err != nil {
				return nil, fmt.Errorf("AggregateSpans(internal): %w", err)
			}
			b, _ := json.Marshal(resp)
			_ = json.Unmarshal(b, &raw)
			s.cache.Set("v2.AggregateSpansInternal", s.site, req, raw)
		}
		// service -> count
		buckets := parseAggregateCounts(raw, "service")
		for k, v := range buckets {
			out[k] += v
		}
	}
	return out, nil
}

//
// ========= パース・集約補助 =========
//

func parseAggregatePairs(raw map[string]any, facetA, facetB string) []DepRow {
	var rows []DepRow
	data, _ := raw["data"].([]any)
	for _, item := range data {
		itm, _ := item.(map[string]any)
		attrs, _ := itm["attributes"].(map[string]any)
		by, _ := attrs["by"].(map[string]any)
		computes, _ := attrs["computes"].(map[string]any)

		var count float64
		for _, v := range computes {
			if m, ok := v.(map[string]any); ok {
				if val, ok := m["value"].(float64); ok {
					count = val
				} else if val, ok := m["sum"].(float64); ok {
					count = val
				}
				break
			}
		}
		a, _ := by[facetA].(string)
		b, _ := by[facetB].(string)
		if a == "" && b == "" {
			continue
		}
		rows = append(rows, DepRow{
			OutgoingResource: a,
			PeerService:      b,
			Count:            count,
		})
	}
	return reduceRows(rows)
}

func parseAggregateCounts(raw map[string]any, facet string) map[string]float64 {
	out := map[string]float64{}
	data, _ := raw["data"].([]any)
	for _, item := range data {
		itm, _ := item.(map[string]any)
		attrs, _ := itm["attributes"].(map[string]any)
		by, _ := attrs["by"].(map[string]any)
		computes, _ := attrs["computes"].(map[string]any)

		k, _ := by[facet].(string)
		if k == "" {
			continue
		}
		for _, v := range computes {
			if m, ok := v.(map[string]any); ok {
				if val, ok := m["value"].(float64); ok {
					out[k] += val
				} else if val, ok := m["sum"].(float64); ok {
					out[k] += val
				}
			}
		}
	}
	return out
}

func reduceRows(in []DepRow) []DepRow {
	type key struct{ r, p string }
	m := map[key]float64{}
	for _, row := range in {
		k := key{row.OutgoingResource, row.PeerService}
		m[k] += row.Count
	}
	out := make([]DepRow, 0, len(m))
	for k, v := range m {
		out = append(out, DepRow{
			OutgoingResource: k.r,
			PeerService:      k.p,
			Count:            v,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].PeerService == out[j].PeerService {
				return out[i].OutgoingResource < out[j].OutgoingResource
			}
			return out[i].PeerService < out[j].PeerService
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func toSortedBuckets(m map[string]float64) []Bucket {
	out := make([]Bucket, 0, len(m))
	for k, v := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, Bucket{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func uniq(ss []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
