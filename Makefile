SHELL := /usr/bin/env bash

.PHONY: all fmt test test-integration-postgres lint proto generate-tools

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
