SHELL := /usr/bin/env bash

.PHONY: all fmt test test-integration-postgres lint proto generate-tools dr-drill perf-qual failover-evidence keyset-evidence soak-qual

all: fmt test

fmt:
	gofmt -w $$(find . -type f -name '*.go' -not -path './gen/*')

test:
	go test ./...

test-integration-postgres:
	RGS_TEST_DATABASE_URL=$${RGS_TEST_DATABASE_URL:?set RGS_TEST_DATABASE_URL} go test ./internal/platform/server -run '^TestPostgres'

lint:
	golangci-lint run

proto:
	buf lint
	buf generate

dr-drill:
	RGS_DATABASE_URL=$${RGS_DATABASE_URL:?set RGS_DATABASE_URL} \
	RGS_DRILL_RESTORE_URL=$${RGS_DRILL_RESTORE_URL:-} \
	./scripts/dr_backup_restore_check.sh

perf-qual:
	RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX=$${RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX:-} \
	./scripts/perf_slo_check.sh

failover-evidence:
	RGS_FAILOVER_EVENT_ID=$${RGS_FAILOVER_EVENT_ID:-} \
	RGS_FAILOVER_OUTAGE_START_UNIX=$${RGS_FAILOVER_OUTAGE_START_UNIX:?set RGS_FAILOVER_OUTAGE_START_UNIX} \
	RGS_FAILOVER_RECOVERY_UNIX=$${RGS_FAILOVER_RECOVERY_UNIX:?set RGS_FAILOVER_RECOVERY_UNIX} \
	RGS_FAILOVER_LAST_DURABLE_UNIX=$${RGS_FAILOVER_LAST_DURABLE_UNIX:?set RGS_FAILOVER_LAST_DURABLE_UNIX} \
	RGS_FAILOVER_RTO_MAX_SECONDS=$${RGS_FAILOVER_RTO_MAX_SECONDS:-} \
	RGS_FAILOVER_RPO_MAX_SECONDS=$${RGS_FAILOVER_RPO_MAX_SECONDS:-} \
	./scripts/failover_evidence_snapshot.sh

keyset-evidence:
	RGS_JWT_KEYSET_FILE=$${RGS_JWT_KEYSET_FILE:-} \
	RGS_JWT_KEYSET_COMMAND=$${RGS_JWT_KEYSET_COMMAND:-} \
	RGS_KEYSET_EVENT_ID=$${RGS_KEYSET_EVENT_ID:-} \
	RGS_KEYSET_PREVIOUS_SUMMARY_FILE=$${RGS_KEYSET_PREVIOUS_SUMMARY_FILE:-} \
	./scripts/keyset_rotation_evidence.sh

soak-qual:
	RGS_SOAK_RUNS=$${RGS_SOAK_RUNS:-3} \
	RGS_SOAK_BENCHTIME=$${RGS_SOAK_BENCHTIME:-30s} \
	RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX=$${RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX:-} \
	RGS_SOAK_WAGER_PLACE_NS_OP_MAX=$${RGS_SOAK_WAGER_PLACE_NS_OP_MAX:-} \
	./scripts/load_soak_check.sh
