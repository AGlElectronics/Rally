# Rally — Events & conventions

Rally is the convention / events module of the Con Services stack.
Owns organisations, editions, memberships, calls for papers with
configurable forms, the public submission site, staff review, the
schedule builder with its public schedule API, and live-day changes with
Telegram announcements. See `docs/` for the full architecture and
`docs/poc-requirements.md` for the current scope; this README is strictly
about getting the stack running locally.

- **Backend:** Go 1.25, chi, pgx (hand-written queries — `sqlc.yaml` is
  wired for a later pass, see the note at the top of that file).
- **Frontend:** React 19 + Rsbuild + TanStack Router/Query + Tailwind.
- **Database:** PostgreSQL 16, migrated via Goose (`cmd/migrate` one-shot).
- **Object storage:** MinIO (via `minio-go` v7) — **LOCAL DEV ONLY**, never
  deploy in production (ADR-0004).
- **Go module:** `github.com/AGlElectronics/rally/backend`.

## Reserved ports

| Service         | Host port | Container port |
| --------------- | --------- | -------------- |
| backend (API)   | **8100**  | 8080           |
| frontend (SPA)  | **3002**  | 80             |
| postgres        | 5435      | 5432           |
| MinIO S3 API    | 9020      | 9000           |
| MinIO console   | 9021      | 9001           |

Every module in this ecosystem binds `:8080` *inside* its container; the
host-side port is reserved per-module (Rally = 8100) and the Compose file
maps `host:8100 -> container:8080`. The host ports here are chosen to avoid
colliding with `stash` (8080/3000) and `hr` (8110/3003).

## Auth modes

Set `AUTH_MODE` on the backend AND the matching `VITE_AUTH_MODE` on the
frontend build:

- `AUTH_MODE=dev` — the backend reads `X-Dev-User` (or `DEV_DEFAULT_USER`)
  and auto-provisions a local user under the reserved `dev` issuer. The
  SPA stamps the header from `localStorage['rally.devUser']`.
- `AUTH_MODE=oidc` — the backend verifies a Bearer JWT via OIDC
  discovery. The SPA uses `oidc-client-ts` to run the authorisation-code
  flow against `VITE_OIDC_AUTHORITY` / `VITE_OIDC_CLIENT_ID`.

`AUTH_MODE=oidc` requires both `OIDC_ISSUER_URL` and `OIDC_CLIENT_ID` to
be set on the backend; the Dockerfile.frontend fails the build if the
matching `VITE_OIDC_*` build-args are missing.

### Attendee accounts (submission site)

People who submit ideas sign in to a **separate** issuer (in the Con
Services stack: the Keycloak realm `attendees`, client `rally-submit`), so a
public account can never act as staff. Set `ATTENDEE_OIDC_ISSUER_URL` and
`ATTENDEE_OIDC_CLIENT_ID` on the backend; the issuer must differ from the
staff one. Without them the public API serves open calls only.

Dev test attendees: `ada@attendee.test` / `attendee` and
`ben@attendee.test` / `attendee`.

## Two frontends from one codebase

`frontend/` builds either the staff app (default) or the public submission
site (`RALLY_APP=submit`, entry `src/submit/main.tsx`, dev port 3004). The
Dockerfile takes `--build-arg RALLY_APP=submit`; `VITE_PRODUCT_NAME` names
the public site. `npm run typecheck` checks both programs.

In the full stack the submission site is served at
`https://submit.conservices.test` (re-run `scripts/setup-local-dns.ps1` as
administrator once so the hostname resolves) and on `http://localhost:3004`.

## Endpoints

Health and branding (no auth): `GET /livez`, `GET /readyz`,
`GET /api/health`, `GET /api/v1/brand`.

Staff API (`/api/v1`, staff token):

| Method | Path | Notes |
| ------ | ---- | ----- |
| GET | `/me`, `/organizations[/{id}]`, `/editions` | |
| GET, POST | `/editions/{edition}/cfps` | `cfp.manage` to create |
| GET, PUT, DELETE | `/cfps/{cfp}` | PUT carries form, review mode, event types, status labels |
| GET, POST | `/editions/{edition}/event-types` | |
| PUT, DELETE | `/event-types/{id}` | |
| GET | `/editions/{edition}/submissions` | `submission.review` |
| GET, PATCH | `/submissions/{id}` | |
| POST | `/submissions/{id}/accept`, `/reject` | `submission.decide`; body `{"note"}` is shown to the submitter |
| GET, POST | `/submissions/{id}/notes` | staff-only thread |
| GET, PUT | `/submissions/{id}/scores` | scored review mode; not on your own submission |
| POST | `/uploads`, `/uploads/{file}/complete` | presigned S3 POST |
| GET | `/files/{file}/download` | short-lived presigned link |
| PUT | `/editions/{edition}/dates` | timezone, first and last day |
| GET, POST | `/editions/{edition}/rooms` | `schedule.edit` to create |
| PUT, DELETE | `/rooms/{room}` | |
| GET | `/editions/{edition}/schedule` | draft, settings, unscheduled submissions, conflicts |
| PUT | `/editions/{edition}/schedule/settings` | grid hours, snap, rule modes |
| POST | `/editions/{edition}/schedule/items` | blocking rules answer 409 with `details.conflicts` |
| PATCH, DELETE | `/schedule-items/{item}` | |
| POST | `/editions/{edition}/schedule/publish` | `schedule.publish`; `{"note"}` |
| GET | `/editions/{edition}/schedule/versions[/{version}]` | |
| POST | `/editions/{edition}/schedule/versions/{version}/restore` | loads it into the draft |
| POST | `/editions/{edition}/schedule/live/{item}` | `{"action":"delay"\|"cancel"\|"restore"\|"move", ...}`; `force` overrides blocks |
| GET, POST | `/editions/{edition}/announcements` | |
| PUT | `/announcements/{id}` | edit a draft |
| POST | `/announcements/{id}/send`, `/discard` | send posts to the enabled chats |
| GET, POST | `/editions/{edition}/telegram-chats` | |
| PUT, DELETE | `/telegram-chats/{chat}` | |

Public schedule (`/api/public/v1`; 404 while switched off, 401 without an
API key):

| Method | Path | Notes |
| ------ | ---- | ----- |
| GET | `/schedules` | editions with a published schedule |
| GET | `/schedules/{edition}` | latest published version; `{edition}` is the id or a unique slug |
| GET | `/schedules/{edition}/calendar.ics` | iCal feed |

Every consumer needs an API key: `Authorization: Bearer <key>`, or `?key=`
for calendar subscriptions (they cannot send headers). Staff with
`schedule.publish` manage per-consumer keys in Schedule → Setup
(`GET/POST /api/v1/public-api-keys`, `DELETE /api/v1/public-api-keys/{id}`);
a key's value is shown once and only its SHA-256 is stored. The submission
site's Programme page uses the builtin "programme-page" key
(`PUBLIC_API_BUILTIN_KEY` on the backend = `VITE_PUBLIC_SCHEDULE_KEY` on the
submit build; it ships to every browser, so it buys rotation and per-client
tracking, not secrecy — change the env value to rotate it).

These send `ETag` and `Cache-Control: private, max-age=60`. The on/off
switch is the Den organisation setting `events_public_schedule`; without it,
`PUBLIC_SCHEDULE_ENABLED` decides. `TELEGRAM_BOT_TOKEN` (secret) lets the
bot post; when it's empty, sending an announcement is a recorded dry run.

Public API (`/api/public/v1`, attendee token unless noted):

| Method | Path | Notes |
| ------ | ---- | ----- |
| GET | `/calls`, `/calls/{cfp}` | no auth; drafts are hidden, closed calls stay readable |
| GET | `/me` | profile and presenter code |
| GET | `/submissions` | mine, as submitter or co-presenter |
| POST | `/calls/{cfp}/submissions` | create a draft |
| GET, PATCH | `/submissions/{id}` | owner edits while the call accepts submissions |
| POST | `/submissions/{id}/submit`, `/withdraw`, `/leave` | |
| POST | `/submissions/{id}/people` | `{"code"}` or `{"email"}`; unknown emails become invite links |
| DELETE | `/submissions/{id}/people/{person}` | |
| POST | `/submissions/{id}/people/{person}/reissue` | new invite link |
| GET | `/invites/{code}` | preview |
| POST | `/invites/{code}/accept`, `/decline` | |
| POST | `/uploads`, `/uploads/{file}/complete` | |
| GET | `/files/{file}/download` | |

Uploads never touch the API server's disk: `POST /uploads` returns a
presigned POST (`url`, `fields`), the browser sends the file straight to
S3, then calls `/complete`, which checks the object's size and type.
Unfinished uploads are cleaned up by a background sweeper.

Error envelope: `{"error":{"code","message","details"}}`. Validation
details are keyed by field, e.g. `answers.<fieldId>` or
`form.fields[2].label`.

## First-time boot

Requirements:

- Docker Desktop (or equivalent) with Compose v2.
- No local processes listening on the host ports above.

```sh
cd Rally
docker compose -f infra/docker-compose.yml up --build
```

What happens:

1. `postgres` boots and reports healthy.
2. `minio` + `minio-init` run; the `events-data-dev` bucket is created.
3. `migrate` (one-shot) applies all pending Goose migrations and exits 0.
4. `backend` starts on container `:8080`, published on host `:8100`. It
   does **not** auto-migrate (ADR-0003).
5. `frontend` builds the SPA, serves it from nginx on `:3002`, and proxies
   `/api/...` + `/livez` + `/readyz` to `backend:8080`.

Health probes:

```sh
curl -s http://localhost:8100/livez
curl -s http://localhost:3002/livez       # via nginx
curl -s -H 'X-Dev-User: dev-admin' http://localhost:8100/api/v1/me
```

## Standalone image build

The Dockerfiles are context-agnostic — a plain

```sh
cd Rally
docker build -f infra/Dockerfile.backend  -t rally-backend  .
docker build -f infra/Dockerfile.frontend -t rally-frontend .
docker build -f infra/Dockerfile.migrate  -t rally-migrate  .
```

works from the `Rally/` directory. All three Dockerfiles expect the build
context to be `Rally/` (not `Rally/backend` or `Rally/frontend`) so they
can see both the source tree and `infra/nginx.conf`.

To build the frontend for an OIDC deployment:

```sh
docker build \
  -f infra/Dockerfile.frontend \
  --build-arg VITE_AUTH_MODE=oidc \
  --build-arg VITE_OIDC_AUTHORITY=https://keycloak.example.com/realms/rally \
  --build-arg VITE_OIDC_CLIENT_ID=rally-spa \
  -t rally-frontend .
```

## Backups

In the full Con Services stack, Den's operator backs up Rally's database and
bucket every night along with everything else (see
[docs/operations.md](../docs/operations.md)).

**Run standalone, or on a database server Den's operator doesn't manage, Rally
is not backed up by anything. That is the administrator's job:** schedule a
`pg_dump --format=custom` of the Rally database and a copy of its bucket,
encrypt both, and keep them somewhere other than the server.

## Working locally without Docker

```sh
# Backend
cd Rally/backend
go mod tidy
DATABASE_URL='postgres://rally:rally@localhost:5435/rally?sslmode=disable' \
  go run ./cmd/migrate
HTTP_ADDR=:8100 \
DATABASE_URL='postgres://rally:rally@localhost:5435/rally?sslmode=disable' \
S3_ENABLED=false AUTH_MODE=dev \
  go run ./cmd/server

# Frontend
cd Rally/frontend
npm install
npm run dev                 # serves on :3002, proxies /api to :8100
```

## Repository layout

```
Rally/
├── backend/
│   ├── cmd/
│   │   ├── migrate/      # one-shot Goose runner (ADR-0003)
│   │   └── server/       # API; NEVER migrates
│   ├── internal/         # app, config, http, service, auth, …
│   ├── migrations/       # embedded via migrations/embed.go
│   └── queries/          # sqlc queries (hand-rolled pgx today)
├── frontend/             # Rsbuild SPA
├── infra/                # Docker Compose stack + Dockerfiles + nginx
├── deploy/kubernetes/    # Helm/Argo manifests (scaffolded, not wired)
└── docs/                 # ADRs + design docs
```
