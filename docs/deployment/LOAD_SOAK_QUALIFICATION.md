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

With explicit CPU profile:

```bash
RGS_SOAK_RUNS=4 \
RGS_SOAK_BENCHTIME=30s \
RGS_SOAK_CPU=2 \
RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX=110000 \
RGS_SOAK_WAGER_PLACE_NS_OP_MAX=170000 \
make soak-qual
```

## Profile Matrix (Jurisdiction/Operator Class)

Run predefined profile classes (small/medium/large) with class-specific concurrency and thresholds:

```bash
make soak-qual-matrix
```

Optional profile selection:

```bash
RGS_SOAK_PROFILE_SET=us-regulated-small,us-regulated-large \
make soak-qual-matrix
```

## Artifacts

Artifacts are written under `${RGS_SOAK_WORKDIR:-/tmp/open-rgs-go-soak}/<UTC timestamp>/`:

- `benchmark_output.txt`
- `summary.json`

Use `summary.json` as the release evidence anchor for pass/fail gating.

Matrix mode artifacts are written under `${RGS_SOAK_MATRIX_WORKDIR:-/tmp/open-rgs-go-soak-matrix}/<UTC timestamp>/`:

- `<profile>/benchmark_output.txt`
- `<profile>/summary.json`
- `matrix_summary.json`

Use `matrix_summary.json` as the operator-class qualification evidence anchor.

## DB-backed Qualification

To validate durability-path throughput with PostgreSQL enabled:

```bash
RGS_SOAK_DATABASE_URL=postgres://user:pass@localhost:5432/open_rgs_go?sslmode=disable \
RGS_SOAK_RUNS=3 \
RGS_SOAK_BENCHTIME=30s \
RGS_SOAK_CPU=2 \
RGS_SOAK_DB_LEDGER_DEPOSIT_NS_OP_MAX=350000 \
RGS_SOAK_DB_WAGER_PLACE_NS_OP_MAX=400000 \
make soak-qual-db
```

DB-backed artifacts are written under `${RGS_SOAK_DB_WORKDIR:-/tmp/open-rgs-go-soak-db}/<UTC timestamp>/`:

- `benchmark_output.txt`
- `summary.json`

Use DB-backed `summary.json` alongside in-memory and profile-matrix artifacts for release sign-off.
