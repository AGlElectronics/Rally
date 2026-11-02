# ADR 0003 — Migrations run as a one-shot Job, never on API start

- **Status:** Accepted
- **Date:** 2026-11-02
- **Deciders:** Rally engineering
- **Related:** `techstack.md` §4 (persistence policy)

## Context

Stash's reference implementation calls `db.MigrateUp` from
`internal/app/app.go` on every API start. That is acceptable for a single
replica, but the ecosystem spec requires Rally to run multiple replicas
and to make a Kubernetes rollout safe:

> Run exactly **one** authorized Goose migration operation per deployment
> (one-off Compose task or Kubernetes Job/release step with concurrency
> control). API replicas must never each auto-run migrations.
> — `techstack.md` §9.4

Multiple replicas racing `goose up` would at best be slow and at worst
corrupt the `goose_db_version` table.

## Decision

- A dedicated binary — `backend/cmd/migrate` — runs Goose using the same
  embedded migrations the API image carries. It is built into its own
  minimal image (`infra/Dockerfile.migrate`).
- `docker compose up`:
  - `migrate` is a `restart: "no"` service that depends on
    `postgres: service_healthy`;
  - `backend` adds `migrate: service_completed_successfully` to its
    `depends_on`, so a developer `up` fails loudly if migrations fail.
- Kubernetes:
  - `deploy/kubernetes/migrate-job.yaml` runs as an Argo CD `PreSync` hook
    (or whatever the deployment system calls "run before rollout").
  - The API Deployment has no `initContainer` that re-runs migrations.

## Consequences

- **+** Rolling restarts and multi-replica scaling are safe without
  additional locks.
- **+** A failed migration blocks rollout instead of leaking an unhealthy
  schema into production.
- **−** Two container images instead of one. Mitigated by sharing the Go
  build cache in CI and reusing the same base image.
- **−** `goose down` is intentionally supported by `cmd/migrate` for dev
  convenience but is explicitly called out as "DEV ONLY — restore from
  backup for destructive rollbacks" in its usage text.
