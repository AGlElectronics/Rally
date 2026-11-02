# ADR 0004 — MinIO is a local-development convenience only

- **Status:** Accepted
- **Date:** 2026-11-02
- **Deciders:** Rally engineering
- **Related:** `techstack.md` §7 (files, attachments)

## Context

Rally's object-storage interface is deliberately provider-neutral: the Go
code imports `internal/files` and does not know which S3-compatible service
is behind it. In local development we ship a MinIO container so a
contributor can `docker compose up` with no cloud account.

The ecosystem spec is explicit that local emulators MUST NOT become a
required production dependency:

> Introduce object storage only when a real feature needs it. Use a storage
> adapter so an installation can select an appropriate supported S3-
> compatible service or other backend; do not couple application code to a
> specific server or local emulator. — `techstack.md` §7

## Decision

- `infra/docker-compose.yml` ships MinIO on ports `9020` / `9021`. It is
  for local dev and CI smoke tests ONLY.
- The default bucket name is `events-data-dev` to signal the environment.
  Production MUST use a different bucket name on a managed service.
- The Kubernetes manifests reference S3 configuration via a `rally-s3`
  Secret. They DO NOT include a MinIO Deployment. Operators point
  `S3_ENDPOINT`/credentials at a managed S3-compatible bucket (AWS S3,
  GCS S3 interop, Backblaze B2, Scaleway, etc.).
- `S3_ENABLED=false` disables the object-storage client entirely so CI
  smoke runs don't need a bucket.

## Consequences

- **+** Zero-friction local dev; no cloud secrets needed to run Rally.
- **+** Production deployments inherit the operator's bucket lifecycle,
  encryption-at-rest, backup, and IAM policies — all of which MinIO would
  not provide.
- **−** The dev→prod storage transition requires a deliberate config change
  per environment (not an environment-blind image). We think this is the
  right trade.
