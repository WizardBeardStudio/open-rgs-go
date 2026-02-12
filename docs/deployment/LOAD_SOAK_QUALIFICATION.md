# Deployment Guide: Load and Soak Qualification

This guide defines sustained benchmark qualification for core hot paths before release.

## Scope

- `BenchmarkLedgerDeposit`
- `BenchmarkWageringPlaceWager`

## Runbook

From `src/`:

```bash
RGS_SOAK_RUNS=3 \
RGS_SOAK_BENCHTIME=30s \
./scripts/load_soak_check.sh
```

With SLO-style thresholds:

```bash
RGS_SOAK_RUNS=3 \
RGS_SOAK_BENCHTIME=30s \
RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX=80000 \
RGS_SOAK_WAGER_PLACE_NS_OP_MAX=120000 \
./scripts/load_soak_check.sh
```

Or with make:

```bash
RGS_SOAK_RUNS=3 \
RGS_SOAK_BENCHTIME=30s \
RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX=80000 \
RGS_SOAK_WAGER_PLACE_NS_OP_MAX=120000 \
make soak-qual
```

## Artifacts

Artifacts are written under `${RGS_SOAK_WORKDIR:-/tmp/open-rgs-go-soak}/<UTC timestamp>/`:

- `benchmark_output.txt`
- `summary.json`

Use `summary.json` as the release evidence anchor for pass/fail gating.
