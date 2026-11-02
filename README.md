# Rally — Events & conventions

Rally is the convention / events module of the Awoostria service stack.
Owns organisations, editions, memberships, and (coming next) the
submission → review → scheduling pipeline. See `docs/` for the full
architecture; this README is strictly about getting the stack running
locally.

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

## Endpoints

| Method | Path                            | Auth |
| ------ | ------------------------------- | ---- |
| GET    | `/livez`                        | none |
| GET    | `/readyz`                       | none |
| GET    | `/api/health`                   | none |
| GET    | `/api/v1/brand`                 | none |
| GET    | `/api/v1/me`                    | required |
| GET    | `/api/v1/organizations`         | required |
| GET    | `/api/v1/organizations/{id}`    | required |

Error envelope: `{"error":{"code","message","details"}}`.

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
