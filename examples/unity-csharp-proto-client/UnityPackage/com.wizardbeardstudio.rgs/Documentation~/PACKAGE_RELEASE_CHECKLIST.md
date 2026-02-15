# Package Release Checklist

Use this checklist before publishing a package build (including Unity Asset Store submission package).

## Versioning and Metadata

- `package.json` version updated.
- `CHANGELOG.md` updated for this version.
- `README.md` and docs reflect current API behavior.
- License file present and valid (`LICENSE.md`).

## Build and Import

- Package imports cleanly in a fresh Unity project (supported versions).
- No compile errors in Runtime or Editor assemblies.
- Samples import cleanly from `Samples~`.

## Functional Validation

- AuthAndBalance sample runs:
  - login succeeds
  - balance call succeeds/denial handled
- QuickStartSlot sample runs:
  - start session
  - place wager with idempotency handling
  - settle wager
  - end session

## Transport Validation

- `GrpcWeb` mode validated.
- `RestGateway` mode validated.
- WebGL build validates REST mode and CORS/header behavior.

## Security and Compliance

- No secrets committed in package assets/docs.
- Request metadata and idempotency expectations documented.
- Denial/error handling guidance documented for integrators.

## Distribution Readiness

- Unity Asset Store package description + screenshots prepared.
- Known limitations section included.
- Support/contact links present.
