# Datadog dashboard

`dashboard.json` defines a 7-widget dashboard charting validation-run health
over time. Live at https://us3.datadoghq.com/dashboard/2n5-36z-3rc/btf2go-validation.

## Recreate / sync the dashboard

Requires `DATADOG_API_KEY` and `DATADOG_APP_KEY` in `.env`.

Create:
```bash
set -a; source .env; set +a
curl -sS -X POST "https://api.${DATADOG_SITE:-datadoghq.com}/api/v1/dashboard" \
  -H "DD-API-KEY: $DATADOG_API_KEY" \
  -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
  -H "Content-Type: application/json" \
  -d @validation/datadog/dashboard.json
```

Update an existing dashboard (replace `<ID>` with the dashboard ID):
```bash
curl -sS -X PUT "https://api.${DATADOG_SITE:-datadoghq.com}/api/v1/dashboard/<ID>" \
  -H "DD-API-KEY: $DATADOG_API_KEY" \
  -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
  -H "Content-Type: application/json" \
  -d @validation/datadog/dashboard.json
```

## Widgets

1. **Latest PASS** — query_value, latest `findings.pass` (green when >0)
2. **Latest FAIL** — query_value, latest `findings.fail` (red when >0, green at 0)
3. **Latest SKIP** — query_value, latest `findings.skip`
4. **Findings over time** — timeseries, pass/fail/skip lines
5. **Per-tier pass rate** — timeseries, grouped by `tier` tag, y-axis 0-1
6. **Per-tier findings total** — bar timeseries, grouped by `tier` tag
7. **Validation runs** — event stream, filtered to `source_type_name:btf2go`

## Source of metrics + events

Emitted by `validation/runner` at the end of each run. Gated on `DATADOG_API_KEY`
env var; silently skipped without it. See `validation/runner/datadog.go`.

---

# Monitors

Three regression-detection monitors committed under `validation/datadog/monitors/`.

| File | Monitor ID | Name | URL |
|------|-----------|------|-----|
| `monitors/any-fail.json` | `18744655` | btf2go validation — any tier FAIL | https://us3.datadoghq.com/monitors/18744655 |
| `monitors/tier-pass-rate.json` | `18744656` | btf2go validation — per-tier pass rate dropped below 100% | https://us3.datadoghq.com/monitors/18744656 |
| `monitors/no-data.json` | `18744673` | btf2go validation — no runs in 7 days | https://us3.datadoghq.com/monitors/18744673 |

## Monitor descriptions

### any-fail (`18744655`)

Fires when `btf2go.validation.findings.fail` is > 0 in any 30-minute window. Priority 3 (warning). Use this as the fast-path alert for regressions.

### tier-pass-rate (`18744656`)

Multi-alert, one alarm per `tier` tag, fires when `btf2go.validation.tier.pass_rate` drops below 1.0 (100%) in any 30-minute window. Priority 3 (warning). Pinpoints which tier is regressing.

### no-data (`18744673`)

Fires when the sum of all `findings.*` metrics stays at or below 0 over a 7-day window, **or** when no data is received for 72 hours (`no_data_timeframe: 4320` — the Datadog API maximum). Priority 4 (info). Catches a completely silent validation framework.

> Note: Datadog caps `no_data_timeframe` at 4320 minutes (72 h). For true 7-day silence detection the metric query window (`last_7d`) is the primary signal; the 72 h no-data guard fires first if metrics stop entirely.

## Create / sync monitors

Requires `DATADOG_API_KEY` and `DATADOG_APP_KEY` in `.env`.

Create all three (run from repo root):
```bash
set -a; source .env; set +a
for f in validation/datadog/monitors/*.json; do
  name=$(jq -r .name "$f")
  id=$(curl -sS -X POST "https://api.${DATADOG_SITE:-datadoghq.com}/api/v1/monitor" \
    -H "DD-API-KEY: $DATADOG_API_KEY" \
    -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
    -H "Content-Type: application/json" \
    -d @"$f" | jq .id)
  echo "Created: $name  id=$id"
done
```

Update an existing monitor (replace `<ID>` and `<file>`):
```bash
curl -sS -X PUT "https://api.${DATADOG_SITE:-datadoghq.com}/api/v1/monitor/<ID>" \
  -H "DD-API-KEY: $DATADOG_API_KEY" \
  -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
  -H "Content-Type: application/json" \
  -d @validation/datadog/monitors/<file>.json
```
