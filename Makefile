SHELL := /usr/bin/env bash

.PHONY: all fmt test test-integration-postgres lint proto generate-tools dr-drill perf-qual

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
