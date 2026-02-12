# Deployment Guide: Performance Qualification

This guide defines baseline performance evidence generation for open-rgs-go.

## Objectives

- Produce repeatable benchmark artifacts per release.
- Track trendline drift in hot-path operations.
- Optionally enforce SLO-style upper bounds for key latency metrics.

## Baseline Benchmark

- `BenchmarkLedgerDeposit` (`internal/platform/server/ledger_benchmark_test.go`)

## Runbook

From `src/`:

```bash
./scripts/perf_slo_check.sh
```

With optional threshold gate:

```bash
RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX=50000 ./scripts/perf_slo_check.sh
```

Or with make:

```bash
RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX=50000 make perf-qual
```

## Artifacts

Artifacts are written under `${RGS_PERF_WORKDIR:-/tmp/open-rgs-go-perf}/<UTC timestamp>/`:

- `benchmark_output.txt`
- `summary.txt`

Attach these to the release evidence packet and compare against prior accepted runs.
