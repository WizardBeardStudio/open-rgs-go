# Report Catalog

## Supported Report Types

### 1) System Significant Events and Alterations
- `report_type`: `REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS`
- Purpose: regulator-facing event timeline for significant events and alterations.
- Primary source data:
  - `significant_events` ingestion stream
  - event metadata (`event_id`, equipment id, event code, severity)
- Required metadata fields in every output:
  - operator identifier
  - report title
  - selected interval
  - generated timestamp
  - no activity indicator
- Output fields (row-level):
  - event id
  - equipment id
  - event code
  - localized description
  - severity
  - occurred at
  - received at
  - recorded at

### 2) Cashless Liability Summary
- `report_type`: `REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY`
- Purpose: summarize current cashless liabilities across tracked accounts.
- Primary source data:
  - ledger accounts and balances
- Required metadata fields in every output:
  - operator identifier
  - report title
  - selected interval
  - generated timestamp
  - no activity indicator
- Output fields (row-level):
  - account id
  - currency
  - available
  - pending
  - total
- Output summary fields:
  - total available
  - total pending

## Supported Intervals
- `REPORT_INTERVAL_DTD`
- `REPORT_INTERVAL_MTD`
- `REPORT_INTERVAL_YTD`
- `REPORT_INTERVAL_LTD`

## Supported Formats
- `REPORT_FORMAT_JSON` (`application/json`)
- `REPORT_FORMAT_CSV` (`text/csv`)

## No Activity Behavior
- If the selected interval has no qualifying rows:
  - `no_activity = true`
  - JSON includes `"note": "No Activity"`
  - CSV includes a `No Activity` row

## API Operations
- `GenerateReport`
- `ListReportRuns`
- `GetReportRun`

## Implementation References
- Proto: `api/proto/rgs/v1/reporting.proto`
- Service: `internal/platform/server/reporting_grpc.go`
- Storage schema: `migrations/000004_reporting_runs.up.sql`
- Tests:
  - `internal/platform/server/reporting_grpc_test.go`
  - `internal/platform/server/reporting_gateway_test.go`
