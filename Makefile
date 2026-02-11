SHELL := /usr/bin/env bash

.PHONY: all fmt test lint proto generate-tools

all: fmt test

fmt:
	gofmt -w $$(find . -type f -name '*.go' -not -path './gen/*')

test:
	go test ./...

lint:
	golangci-lint run

proto:
	buf lint
	buf generate
