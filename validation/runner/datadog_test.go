package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// makeTestRunInfo returns a minimal RunInfo for testing.
func makeTestRunInfo() RunInfo {
	return RunInfo{
		ID:        "2026-01-01T00-00-00.000000000Z-abc1234-all-local",
		Timestamp: "2026-01-01T00:00:00Z",
		Btf2go: Btf2goInfo{
			Version: "v0.3.0",
			Commit:  "abc1234",
			Tag:     "v0.3.0",
			Dirty:   false,
		},
		Params: ParamsInfo{
			Tiers: []string{"all"},
		},
		Environment: EnvironmentInfo{
			Kind: "local",
			Host: "testhost",
			Arch: "amd64",
		},
		Headline: HeadlineInfo{Pass: 5, Fail: 2, Skip: 1, Tiers: 2},
	}
}

func makeTestResults() []TierResult {
	return []TierResult{
		{
			Tier: "T1",
			Findings: []Finding{
				{Project: "a", Status: StatusPass},
				{Project: "b", Status: StatusFail},
			},
		},
		{
			Tier: "T2",
			Findings: []Finding{
				{Project: "c", Status: StatusPass},
				{Project: "d", Status: StatusSkip},
			},
		},
	}
}

// TestEmitToDatadog_WithKey verifies that when DATADOG_API_KEY is set,
// emitToDatadog POSTs to both /api/v2/series and /api/v1/events with
// correct payload shapes and tags.
func TestEmitToDatadog_WithKey(t *testing.T) {
	type request struct {
		path string
		body []byte
	}
	var reqs []request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, request{path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// Override the base URL to point at our test server.
	orig := datadogBaseURL
	datadogBaseURL = srv.URL
	defer func() { datadogBaseURL = orig }()

	t.Setenv("DATADOG_API_KEY", "test-key-123")
	// Unset site so we rely on the override.
	t.Setenv("DATADOG_SITE", "")

	info := makeTestRunInfo()
	results := makeTestResults()

	err := emitToDatadog(info, results)
	if err != nil {
		t.Fatalf("emitToDatadog returned non-nil: %v", err)
	}

	if len(reqs) != 2 {
		t.Fatalf("expected 2 HTTP requests (series + event), got %d", len(reqs))
	}

	// --- Series request ---
	var seriesReq *request
	var eventReq *request
	for i := range reqs {
		if strings.Contains(reqs[i].path, "/api/v2/series") {
			seriesReq = &reqs[i]
		} else if strings.Contains(reqs[i].path, "/api/v1/events") {
			eventReq = &reqs[i]
		}
	}
	if seriesReq == nil {
		t.Fatal("no POST to /api/v2/series")
	}
	if eventReq == nil {
		t.Fatal("no POST to /api/v1/events")
	}

	// Verify series payload has expected metric names.
	var seriesPayload map[string]interface{}
	if err := json.Unmarshal(seriesReq.body, &seriesPayload); err != nil {
		t.Fatalf("series payload is not valid JSON: %v", err)
	}
	seriesList, ok := seriesPayload["series"].([]interface{})
	if !ok || len(seriesList) == 0 {
		t.Fatal("series payload missing 'series' array")
	}

	// Collect all metric names.
	metricNames := map[string]bool{}
	for _, raw := range seriesList {
		s, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := s["metric"].(string)
		metricNames[name] = true

		// Verify every metric has a 'tags' field.
		tags, ok := s["tags"].([]interface{})
		if !ok || len(tags) == 0 {
			t.Errorf("metric %q is missing tags", name)
		}

		// Verify env tag is present.
		hasEnvTag := false
		for _, tRaw := range tags {
			if tag, ok := tRaw.(string); ok && strings.HasPrefix(tag, "env:") {
				hasEnvTag = true
			}
		}
		if !hasEnvTag {
			t.Errorf("metric %q is missing env: tag", name)
		}
	}

	requiredMetrics := []string{
		"btf2go.validation.findings.pass",
		"btf2go.validation.findings.fail",
		"btf2go.validation.findings.skip",
		"btf2go.validation.tier.pass_rate",
		"btf2go.validation.tier.findings_total",
	}
	for _, m := range requiredMetrics {
		if !metricNames[m] {
			t.Errorf("missing required metric %q; got: %v", m, metricNames)
		}
	}

	// --- Event request ---
	var eventPayload map[string]interface{}
	if err := json.Unmarshal(eventReq.body, &eventPayload); err != nil {
		t.Fatalf("event payload is not valid JSON: %v", err)
	}
	title, _ := eventPayload["title"].(string)
	if title == "" {
		t.Error("event payload missing 'title'")
	}
	if !strings.Contains(title, "btf2go validation run") {
		t.Errorf("event title %q doesn't contain expected prefix", title)
	}
	alertType, _ := eventPayload["alert_type"].(string)
	// Fail count is 2, so expect "warning".
	if alertType != "warning" {
		t.Errorf("alert_type = %q, want 'warning' (there are fails)", alertType)
	}
	sourceType, _ := eventPayload["source_type_name"].(string)
	if sourceType != "btf2go" {
		t.Errorf("source_type_name = %q, want 'btf2go'", sourceType)
	}
}

// TestEmitToDatadog_NoKey verifies that when DATADOG_API_KEY is unset,
// emitToDatadog returns nil immediately and makes zero HTTP calls.
func TestEmitToDatadog_NoKey(t *testing.T) {
	var callCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := datadogBaseURL
	datadogBaseURL = srv.URL
	defer func() { datadogBaseURL = orig }()

	t.Setenv("DATADOG_API_KEY", "")

	err := emitToDatadog(makeTestRunInfo(), makeTestResults())
	if err != nil {
		t.Fatalf("emitToDatadog returned non-nil without API key: %v", err)
	}
	if n := callCount.Load(); n != 0 {
		t.Errorf("expected 0 HTTP calls without API key, got %d", n)
	}
}

// TestEmitToDatadog_5xxNonFatal verifies that a 5xx response from
// Datadog logs to stderr but does NOT cause emitToDatadog to return
// a non-nil error (run must continue).
func TestEmitToDatadog_5xxNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := datadogBaseURL
	datadogBaseURL = srv.URL
	defer func() { datadogBaseURL = orig }()

	t.Setenv("DATADOG_API_KEY", "test-key-5xx")

	// Capture log output to verify the error is logged.
	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origOutput) })

	err := emitToDatadog(makeTestRunInfo(), makeTestResults())
	if err != nil {
		t.Fatalf("emitToDatadog must return nil even on 5xx, got: %v", err)
	}

	logged := logBuf.String()
	if !strings.Contains(logged, "[datadog ERROR]") {
		t.Errorf("expected [datadog ERROR] log line on 5xx, got: %q", logged)
	}
}

// TestEmitToDatadog_4xxIncludesResponseBody verifies that when Datadog returns
// a 4xx status, the log output includes both the status code AND a substring
// of the response body — so debugging doesn't require re-running with curl.
//
// This is a regression test for the PR #54 v2-payload bug: the 400 was logged
// but the body ("type field must be integer not string") was not.
func TestEmitToDatadog_4xxIncludesResponseBody(t *testing.T) {
	const responseBody = `{"errors":["bad payload"]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(responseBody))
	}))
	defer srv.Close()

	orig := datadogBaseURL
	datadogBaseURL = srv.URL
	defer func() { datadogBaseURL = orig }()

	t.Setenv("DATADOG_API_KEY", "test-key-4xx")

	// Capture log output.
	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origOutput) })

	err := emitToDatadog(makeTestRunInfo(), makeTestResults())
	if err != nil {
		t.Fatalf("emitToDatadog must return nil even on 4xx (non-fatal), got: %v", err)
	}

	logged := logBuf.String()

	// Must contain the status code.
	if !strings.Contains(logged, "400") {
		t.Errorf("expected log to contain '400', got: %q", logged)
	}

	// Must contain a snippet of the response body — the key new behaviour.
	if !strings.Contains(logged, "bad payload") {
		t.Errorf("expected log to contain 'bad payload' (from response body), got: %q", logged)
	}

	// Must use the louder [datadog ERROR] prefix.
	if !strings.Contains(logged, "[datadog ERROR]") {
		t.Errorf("expected log prefix '[datadog ERROR]', got: %q", logged)
	}
}

// TestBuildSeriesPayload is a pure unit test for the payload builder.
// It verifies metric count and JSON structure without any HTTP.
func TestBuildSeriesPayload(t *testing.T) {
	info := makeTestRunInfo()
	results := makeTestResults()

	data, err := buildSeriesPayload(info, results)
	if err != nil {
		t.Fatalf("buildSeriesPayload: unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("buildSeriesPayload: invalid JSON: %v\nbody: %s", err, data)
	}
	series, ok := payload["series"].([]interface{})
	if !ok {
		t.Fatal("missing 'series' key")
	}

	// 3 global metrics + (2 tiers × 2 per-tier metrics) = 7 total.
	want := 3 + len(results)*2
	if len(series) != want {
		t.Errorf("series count = %d, want %d", len(series), want)
	}
}

// TestBuildSeriesPayload_V2Shape is a regression test for the v2 payload format.
// It asserts that:
//   - "type" is an integer (specifically 3 for gauge) — not a string like "gauge"
//   - "points" are objects with "timestamp" and "value" keys — not [ts, val] arrays
//
// This guards against re-introducing the v1-style body that caused HTTP 400 from
// POST /api/v2/series (confirmed against api.us3.datadoghq.com).
func TestBuildSeriesPayload_V2Shape(t *testing.T) {
	data, err := buildSeriesPayload(makeTestRunInfo(), makeTestResults())
	if err != nil {
		t.Fatalf("buildSeriesPayload: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	series, _ := payload["series"].([]interface{})
	if len(series) == 0 {
		t.Fatal("no series in payload")
	}

	for i, raw := range series {
		s, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("series[%d] is not an object", i)
		}
		metricName, _ := s["metric"].(string)

		// "type" must be a JSON number (float64 after Unmarshal), not a string.
		typeVal, exists := s["type"]
		if !exists {
			t.Errorf("series[%d] (%q) missing 'type' field", i, metricName)
			continue
		}
		typeNum, isFloat := typeVal.(float64)
		if !isFloat {
			t.Errorf("series[%d] (%q) 'type' is %T (%v), want integer (float64 after JSON unmarshal)", i, metricName, typeVal, typeVal)
		} else if typeNum != 3 {
			t.Errorf("series[%d] (%q) 'type' = %v, want 3 (gauge)", i, metricName, typeNum)
		}

		// "points" must be an array of objects with "timestamp" and "value" keys.
		points, ok := s["points"].([]interface{})
		if !ok || len(points) == 0 {
			t.Errorf("series[%d] (%q) 'points' is missing or empty", i, metricName)
			continue
		}
		for j, pRaw := range points {
			pt, ok := pRaw.(map[string]interface{})
			if !ok {
				t.Errorf("series[%d] (%q) points[%d] is %T, want object (v2 requires {timestamp,value})", i, metricName, j, pRaw)
				continue
			}
			if _, hasTS := pt["timestamp"]; !hasTS {
				t.Errorf("series[%d] (%q) points[%d] missing 'timestamp' key", i, metricName, j)
			}
			if _, hasVal := pt["value"]; !hasVal {
				t.Errorf("series[%d] (%q) points[%d] missing 'value' key", i, metricName, j)
			}
		}
	}
}

// TestBuildEventPayload verifies the event JSON structure.
func TestBuildEventPayload(t *testing.T) {
	info := makeTestRunInfo()

	data, err := buildEventPayload(info)
	if err != nil {
		t.Fatalf("buildEventPayload: unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("buildEventPayload: invalid JSON: %v", err)
	}

	// Required fields.
	for _, field := range []string{"title", "text", "tags", "alert_type", "source_type_name"} {
		if payload[field] == nil {
			t.Errorf("event payload missing field %q", field)
		}
	}

	// Tags must include commit:<sha>.
	tags, _ := payload["tags"].([]interface{})
	hasCommit := false
	for _, tRaw := range tags {
		if tag, ok := tRaw.(string); ok && strings.HasPrefix(tag, "commit:") {
			hasCommit = true
		}
	}
	if !hasCommit {
		t.Errorf("event tags missing commit:<sha>; got: %v", tags)
	}
}
