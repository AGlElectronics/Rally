# ADR 0001 — Initial scaffold layout

- **Status:** Accepted
- **Date:** 2026-11-02
- **Deciders:** Rally engineering (initial scaffold)
- **Supersedes:** —
- **Related:** `architecture.md`, `techstack.md`, `delivery-plan.md`

## Context

Rally is the first implementation of the Events module in the modular
convention ecosystem. The ecosystem documents mandate a shared stack (Go
1.25 / Chi / pgx / Goose / React 19 / Rsbuild / TanStack / Tailwind) and a
modular monolith per application; they do not prescribe repository layout,
sqlc adoption timing, or how to shape a repository that must stay buildable
as a day-one scaffold.

The Stash reference lays out `backend/{cmd,internal,migrations,queries}`
and `frontend/src/{api,auth,components,pages,hooks,lib}` with sqlc
generating code at build time. We want the same shape without inheriting
Stash's inventory-specific endpoints, roles, or OIDC bindings schema.

## Decision

1. Mirror the Stash directory layout exactly for backend and frontend. The
   repository folder is `Rally/` but the Go module path is
   `github.com/AGlElectronics/rally/backend` and internal packages use the
   neutral `events` vocabulary (`internal/events`, "edition", "membership"),
   not product-brand terms.
2. Scaffold with hand-written `pgx` queries (`internal/events/events.go`,
   `internal/auth/resolver.go`). Keep `sqlc.yaml` and `queries/` in the tree
   so the first vertical slice can switch to generated code with one `sqlc
   generate` and no layout churn.
3. Keep the HTTP layer minimal: health probes, `/api/v1/brand` (public),
   `/api/v1/me`, `/api/v1/organizations{,/{id}}`. These are enough to prove
   the auth plumbing works end-to-end from the SPA without pretending the
   programme-management features exist yet.
4. Access config lives in a single TOML file, loaded once at startup
   (`internal/auth/accessconfig.go`). The scaffold ships a stub that
   validates the file exists and uses the built-in role catalog; wiring the
   TOML parser is a one-file follow-up with no caller changes.
5. Reserve neutral `brand_source=standalone|den` from day one so the Den
   pairing client can be dropped in later without a config schema change.

## Consequences

- **+** Repo is buildable immediately with `go build ./...` and
  `npm run build`; no toolchain prereqs (sqlc, protoc).
- **+** Switching to sqlc is an isolated change — queries are already in
  `queries/*.sql` with sqlc annotations.
- **−** Hand-written queries duplicate shape between `.sql` and Go code
  for the two foundation endpoints. We accept this trade for scaffold
  simplicity; the first vertical slice collapses the duplication.
- **−** The access config parser is a stub. A startup log line makes this
  obvious to operators; the file must still exist (typo guard).

## Follow-ups

- Replace the access config stub with a real TOML parser once the role
  catalog settles (expected in Phase 2, delivery-plan.md §4).
- Switch to sqlc when adding the first write endpoints (Phase 3).
