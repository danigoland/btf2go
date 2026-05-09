package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// datadogBaseURL is the base URL for all Datadog API calls.
// Overridden in tests to point at an httptest server.
var datadogBaseURL = ""

// datadogBase returns the effective base URL: the test override if set,
// otherwise derived from DATADOG_SITE (defaults to datadoghq.com).
func datadogBase() string {
	if datadogBaseURL != "" {
		return datadogBaseURL
	}
	site := os.Getenv("DATADOG_SITE")
	if site == "" {
		site = "datadoghq.com"
	}
	return "https://api." + site
}

// emitToDatadog posts gauge metrics and an event to Datadog after a
// validation run. It is always non-fatal: errors are logged to stderr
// and the function returns nil so the caller can always ignore the
// return value.
//
// Silently skips if DATADOG_API_KEY is unset.
func emitToDatadog(info RunInfo, results []TierResult) error {
	apiKey := os.Getenv("DATADOG_API_KEY")
	if apiKey == "" {
		return nil
	}

	base := datadogBase()

	// POST gauge metrics to v2 series endpoint.
	seriesBody := buildSeriesPayload(info, results)
	if err := ddPost(base+"/api/v2/series", apiKey, seriesBody); err != nil {
		log.Printf("[datadog] series: %v", err)
	}

	// POST event to v1 events endpoint.
	eventBody := buildEventPayload(info)
	if err := ddPost(base+"/api/v1/events", apiKey, eventBody); err != nil {
		log.Printf("[datadog] event: %v", err)
	}

	return nil
}

// ddPost performs a single HTTP POST and returns an error for non-2xx
// responses or transport failures.
func ddPost(url, apiKey string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s: status %d", url, resp.StatusCode)
	}
	return nil
}

// commonTags builds the standard tag set applied to every metric and
// event: env, host, arch, tag (git describe), dirty.
// commit:<sha> is intentionally omitted from metrics (too high cardinality).
func commonTags(info RunInfo) []string {
	tagStr := info.Btf2go.Tag
	if tagStr == "" {
		tagStr = info.Btf2go.Version
	}
	dirty := "false"
	if info.Btf2go.Dirty {
		dirty = "true"
	}
	arch := info.Environment.Arch
	if arch == "" {
		arch = "unknown"
	}
	return []string{
		"env:" + info.Environment.Kind,
		"host:" + info.Environment.Host,
		"arch:" + arch,
		"tag:" + tagStr,
		"dirty:" + dirty,
	}
}

// ddPoint is a single [timestamp, value] pair for the v2 series API.
type ddPoint [2]interface{}

// ddSeries is one metric series in the v2 payload.
type ddSeries struct {
	Metric string    `json:"metric"`
	Type   string    `json:"type"` // "gauge"
	Points []ddPoint `json:"points"`
	Tags   []string  `json:"tags"`
}

// ddSeriesPayload is the top-level object for POST /api/v2/series.
type ddSeriesPayload struct {
	Series []ddSeries `json:"series"`
}

// buildSeriesPayload constructs the JSON body for the v2 series POST.
// It emits:
//   - btf2go.validation.findings.{pass,fail,skip}  (3 global gauges)
//   - btf2go.validation.tier.pass_rate             (one per tier)
//   - btf2go.validation.tier.findings_total         (one per tier)
func buildSeriesPayload(info RunInfo, results []TierResult) []byte {
	now := time.Now().Unix()
	tags := commonTags(info)

	var series []ddSeries

	// Global findings counters.
	series = append(series,
		ddSeries{
			Metric: "btf2go.validation.findings.pass",
			Type:   "gauge",
			Points: []ddPoint{{now, float64(info.Headline.Pass)}},
			Tags:   tags,
		},
		ddSeries{
			Metric: "btf2go.validation.findings.fail",
			Type:   "gauge",
			Points: []ddPoint{{now, float64(info.Headline.Fail)}},
			Tags:   tags,
		},
		ddSeries{
			Metric: "btf2go.validation.findings.skip",
			Type:   "gauge",
			Points: []ddPoint{{now, float64(info.Headline.Skip)}},
			Tags:   tags,
		},
	)

	// Per-tier metrics with a tier:<T> tag added.
	for _, r := range results {
		tierTags := append(append([]string(nil), tags...), "tier:"+strings.ToLower(r.Tier))
		series = append(series,
			ddSeries{
				Metric: "btf2go.validation.tier.pass_rate",
				Type:   "gauge",
				Points: []ddPoint{{now, r.PassRate()}},
				Tags:   tierTags,
			},
			ddSeries{
				Metric: "btf2go.validation.tier.findings_total",
				Type:   "gauge",
				Points: []ddPoint{{now, float64(len(r.Findings))}},
				Tags:   tierTags,
			},
		)
	}

	payload := ddSeriesPayload{Series: series}
	data, _ := json.Marshal(payload)
	return data
}

// ddEventPayload is the body for POST /api/v1/events.
type ddEventPayload struct {
	Title          string   `json:"title"`
	Text           string   `json:"text"`
	Tags           []string `json:"tags"`
	AlertType      string   `json:"alert_type"`
	SourceTypeName string   `json:"source_type_name"`
}

// buildEventPayload constructs the JSON body for the v1 events POST.
func buildEventPayload(info RunInfo) []byte {
	h := info.Headline
	title := fmt.Sprintf("btf2go validation run on %s: %d pass / %d fail / %d skip",
		info.Environment.Kind, h.Pass, h.Fail, h.Skip)

	text := strings.Join([]string{
		fmt.Sprintf("**Run ID:** %s", info.ID),
		fmt.Sprintf("**Commit:** %s (%s)", info.Btf2go.Commit, info.Btf2go.Version),
		fmt.Sprintf("**Kernel:** %s", info.Environment.Kernel),
		fmt.Sprintf("**Report:** validation/reports/%s.md", info.ID),
		fmt.Sprintf("**Tiers:** %d | Pass: %d | Fail: %d | Skip: %d",
			h.Tiers, h.Pass, h.Fail, h.Skip),
	}, "\n")

	// Event tags = common tags + commit:<sha> (fine for events, high-cardinality OK).
	tags := append(commonTags(info), "commit:"+info.Btf2go.Commit)

	alertType := "info"
	if h.Fail > 0 {
		alertType = "warning"
	}

	payload := ddEventPayload{
		Title:          title,
		Text:           text,
		Tags:           tags,
		AlertType:      alertType,
		SourceTypeName: "btf2go",
	}
	data, _ := json.Marshal(payload)
	return data
}
