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
