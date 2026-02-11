package server

import (
	"context"
	"database/sql"
	"strings"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func (s *ReportingService) persistReportRun(ctx context.Context, meta *rgsv1.RequestMeta, run *rgsv1.ReportRun) error {
	if s == nil || s.db == nil || run == nil {
		return nil
	}
	actorID, actorType := "", ""
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	const q = `
INSERT INTO report_runs (
  report_run_id, report_type, report_interval, report_format, status, operator_id,
  report_title, generated_at, no_activity, content_type, content, request_id, actor_id, actor_type
)
VALUES (
  $1,$2,$3,$4,$5::report_run_status,$6,$7,$8::timestamptz,$9,$10,$11,$12,$13,$14
)
ON CONFLICT (report_run_id) DO UPDATE SET
  status = EXCLUDED.status,
  operator_id = EXCLUDED.operator_id,
  report_title = EXCLUDED.report_title,
  generated_at = EXCLUDED.generated_at,
  no_activity = EXCLUDED.no_activity,
  content_type = EXCLUDED.content_type,
  content = EXCLUDED.content,
  request_id = EXCLUDED.request_id,
  actor_id = EXCLUDED.actor_id,
  actor_type = EXCLUDED.actor_type
`
	_, err := s.db.ExecContext(ctx, q,
		run.ReportRunId,
		reportTypeToDB(run.ReportType),
		reportIntervalToDB(run.Interval),
		reportFormatToDB(run.Format),
		reportStatusToDB(run.Status),
		run.OperatorId,
		run.ReportTitle,
		nonEmptyTime(run.GeneratedAt),
		run.NoActivity,
		run.ContentType,
		run.Content,
		requestID(meta),
		actorID,
		actorType,
	)
	return err
}

func (s *ReportingService) listReportRunsFromDB(ctx context.Context, filter rgsv1.ReportType, limit, offset int) ([]*rgsv1.ReportRun, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	reportType := reportTypeToDB(filter)
	if filter == rgsv1.ReportType_REPORT_TYPE_UNSPECIFIED {
		reportType = ""
	}
	const q = `
SELECT report_run_id, report_type, report_interval, report_format, status::text,
       operator_id, report_title, generated_at, no_activity, content_type, content
FROM report_runs
WHERE ($1 = '' OR report_type = $1)
ORDER BY generated_at DESC, report_run_id DESC
LIMIT $2 OFFSET $3
`
	rows, err := s.db.QueryContext(ctx, q, reportType, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.ReportRun, 0, limit)
	for rows.Next() {
		var (
			runID, typ, interval, format, status, operatorID, title, contentType string
			generatedAt                                                          time.Time
			noActivity                                                           bool
			content                                                              []byte
		)
		if err := rows.Scan(
			&runID, &typ, &interval, &format, &status,
			&operatorID, &title, &generatedAt, &noActivity, &contentType, &content,
		); err != nil {
			return nil, err
		}
		out = append(out, &rgsv1.ReportRun{
			ReportRunId: runID,
			ReportType:  reportTypeFromDB(typ),
			Interval:    reportIntervalFromDB(interval),
			Format:      reportFormatFromDB(format),
			Status:      reportStatusFromDB(status),
			OperatorId:  operatorID,
			ReportTitle: title,
			GeneratedAt: generatedAt.UTC().Format(time.RFC3339Nano),
			NoActivity:  noActivity,
			ContentType: contentType,
			Content:     content,
		})
	}
	return out, rows.Err()
}

func (s *ReportingService) getReportRunFromDB(ctx context.Context, reportRunID string) (*rgsv1.ReportRun, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT report_run_id, report_type, report_interval, report_format, status::text,
       operator_id, report_title, generated_at, no_activity, content_type, content
FROM report_runs
WHERE report_run_id = $1
`
	var (
		runID, typ, interval, format, status, operatorID, title, contentType string
		generatedAt                                                          time.Time
		noActivity                                                           bool
		content                                                              []byte
	)
	err := s.db.QueryRowContext(ctx, q, reportRunID).Scan(
		&runID, &typ, &interval, &format, &status,
		&operatorID, &title, &generatedAt, &noActivity, &contentType, &content,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &rgsv1.ReportRun{
		ReportRunId: runID,
		ReportType:  reportTypeFromDB(typ),
		Interval:    reportIntervalFromDB(interval),
		Format:      reportFormatFromDB(format),
		Status:      reportStatusFromDB(status),
		OperatorId:  operatorID,
		ReportTitle: title,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339Nano),
		NoActivity:  noActivity,
		ContentType: contentType,
		Content:     content,
	}, nil
}

func reportTypeToDB(v rgsv1.ReportType) string {
	switch v {
	case rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS:
		return "significant_events_alterations"
	case rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY:
		return "cashless_liability_summary"
	default:
		return "unknown"
	}
}

func reportTypeFromDB(v string) rgsv1.ReportType {
	switch v {
	case "significant_events_alterations":
		return rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS
	case "cashless_liability_summary":
		return rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY
	default:
		return rgsv1.ReportType_REPORT_TYPE_UNSPECIFIED
	}
}

func reportIntervalToDB(v rgsv1.ReportInterval) string {
	switch v {
	case rgsv1.ReportInterval_REPORT_INTERVAL_DTD:
		return "dtd"
	case rgsv1.ReportInterval_REPORT_INTERVAL_MTD:
		return "mtd"
	case rgsv1.ReportInterval_REPORT_INTERVAL_YTD:
		return "ytd"
	case rgsv1.ReportInterval_REPORT_INTERVAL_LTD:
		return "ltd"
	default:
		return "ltd"
	}
}

func reportIntervalFromDB(v string) rgsv1.ReportInterval {
	switch v {
	case "dtd":
		return rgsv1.ReportInterval_REPORT_INTERVAL_DTD
	case "mtd":
		return rgsv1.ReportInterval_REPORT_INTERVAL_MTD
	case "ytd":
		return rgsv1.ReportInterval_REPORT_INTERVAL_YTD
	case "ltd":
		return rgsv1.ReportInterval_REPORT_INTERVAL_LTD
	default:
		return rgsv1.ReportInterval_REPORT_INTERVAL_UNSPECIFIED
	}
}

func reportFormatToDB(v rgsv1.ReportFormat) string {
	switch v {
	case rgsv1.ReportFormat_REPORT_FORMAT_JSON:
		return "json"
	case rgsv1.ReportFormat_REPORT_FORMAT_CSV:
		return "csv"
	default:
		return "json"
	}
}

func reportFormatFromDB(v string) rgsv1.ReportFormat {
	switch v {
	case "json":
		return rgsv1.ReportFormat_REPORT_FORMAT_JSON
	case "csv":
		return rgsv1.ReportFormat_REPORT_FORMAT_CSV
	default:
		return rgsv1.ReportFormat_REPORT_FORMAT_UNSPECIFIED
	}
}

func reportStatusToDB(v rgsv1.ReportRunStatus) string {
	switch v {
	case rgsv1.ReportRunStatus_REPORT_RUN_STATUS_COMPLETED:
		return "completed"
	case rgsv1.ReportRunStatus_REPORT_RUN_STATUS_FAILED:
		return "failed"
	default:
		return "completed"
	}
}

func reportStatusFromDB(v string) rgsv1.ReportRunStatus {
	switch v {
	case "completed":
		return rgsv1.ReportRunStatus_REPORT_RUN_STATUS_COMPLETED
	case "failed":
		return rgsv1.ReportRunStatus_REPORT_RUN_STATUS_FAILED
	default:
		return rgsv1.ReportRunStatus_REPORT_RUN_STATUS_UNSPECIFIED
	}
}

func nonEmptyTime(v string) string {
	if strings.TrimSpace(v) == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return v
}
