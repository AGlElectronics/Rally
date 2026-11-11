# Changelog

All notable changes to Rally (Events) are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

Needs Den: 1.0.0 or later (Den MAJOR 1).

First release, `1.0.0`. Released versions are tagged `vMAJOR.MINOR.PATCH`;
the images `ghcr.io/aglelectronics/rally-backend`, `rally-frontend`,
`rally-migrate` and `rally-submit` are published as
`MAJOR.MINOR.PATCH-BUILD`, where `BUILD` counts rebuilds of the same
version. Rebuilds get no entry here.

### Added

- Calls for papers with their own forms, staff review (comments and
  scores) and decisions, for the programme team (`events-admin`,
  `events-organizer`, `events-programme`) and reviewers
  (`events-reviewer`).
- Public submission site (`rally-submit`) where attendees sign in with an
  attendee account (a separate realm), propose talks and workshops, invite
  co-presenters and see the programme. It has its own `/config.json`
  (attendee realm, client `rally-submit`, `publicScheduleKey`).
- Schedule builder with published versions, and a public schedule API that
  websites and info screens read with an API key.
- Live-day changes (delays, cancellations) with Telegram announcements.
- Submission attachments in S3; browsers upload and download directly with
  short-lived signed links.
- `DEN_MODE=required` keeps the module locked until it is paired with Den:
  the API answers `503 den_pairing_required` (except health, brand, sign-in
  and the Den connection routes) and the public `GET /api/den/state`
  (also at `/api/v1/den/state`) reports `{"mode","paired"}`. The default
  `standalone` runs without Den; any other value stops the backend at
  startup.
- The web app shows a "Connect this module to Den" page (admins) or
  "Waiting for Den" (everyone else) while the backend runs with
  `DEN_MODE=required` and is not paired yet.
- `TRUSTED_PROXY_CIDRS` (optional): comma-separated CIDRs of the proxies in
  front of the backend whose `X-Forwarded-For` entries are believed. The
  default, `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8,::1/128,fc00::/7`,
  covers the Gateway, nginx and Caddy; change it only for a proxy outside
  the private ranges, or set `none` when no proxy is in front. See
  [Client IP addresses](https://aglelectronics.github.io/con-services/operations/production/#client-ip-addresses).
- The backend can serve HTTPS. Set `TLS_CERT_FILE` and `TLS_KEY_FILE` to a
  PEM certificate and private key; the backend re-reads both every minute,
  so a certificate renewed by cert-manager is used without a restart. With
  neither set it serves plain HTTP, which is what Docker Compose
  development uses. See
  [Internal TLS](https://aglelectronics.github.io/con-services/operations/internal-tls/).
- The web apps read their installation settings (sign-in mode, issuer,
  client id, scopes, API address, product name and, on the submission site,
  the public schedule key) from `/config.json` when they start, so the
  published `rally-frontend` and `rally-submit` images work for every
  installation. nginx serves the file from `/etc/conservices/config.json`
  with `Cache-Control: no-store`, so a change shows on the next page load.
  Without a mounted file the web app shows "Not configured yet"; a broken
  file gives a clear error page. See
  [Web app settings](https://aglelectronics.github.io/con-services/modules/rally/configuration/#web-app-settings-configjson).
- Kubernetes: Helm chart `deploy/charts/rally` `1.0.0` in `con-services`,
  released with the tag `rally/v1.0.0`. Internal TLS is on by default:
  cert-manager issues `rally-backend-tls` and `rally-frontend-tls`, the
  backend and the web app serve HTTPS inside the cluster (web app Service
  on port 443, backend on 8080, both named `https`), and every in-cluster
  address uses `https://`. The cluster needs cert-manager, trust-manager
  and the platform's `conservices-internal-ca` issuer; without them set
  `internalTLS.enabled: false`. See
  [Set up Rally](https://aglelectronics.github.io/con-services/modules/rally/setup/).
- Development: `infra/docker-compose.yml` with SeaweedFS as the S3 server
  (`localhost:9020`, bucket `events-data-dev`), and
  `infra/frontend-config/compose.json` (dev sign-in) mounted as the web
  app's `/config.json`; set `FRONTEND_CONFIG` in `.env` to use another
  file. Building from source needs Go 1.27.3 or newer.
- CI on every pull request (tests, lint, Trivy scans) and signed images with
  an SBOM published to GHCR on version tags (linux/amd64; linux/arm64
  optional).

### Security

- The backend reads the client address from `X-Forwarded-For` only when
  the connection comes from a trusted proxy (`TRUSTED_PROXY_CIDRS`), and
  ignores `X-Real-IP` and `True-Client-IP`.
- On Kubernetes the database URL Den shows ends in
  `?sslmode=verify-full&sslrootcert=/etc/conservices-internal-ca/ca.crt`;
  the platform's PostgreSQL refuses connections without TLS.
- No image runs as root. The web apps run on
  `nginxinc/nginx-unprivileged:1.30.6-alpine` as UID 101 and listen on
  port 8080. All images are built with Go 1.27.3 and pass the CI image
  scan (Trivy, HIGH and CRITICAL issues that have a fix).
- A production build never falls back to dev sign-in: a `/config.json`
  without `authMode` means OIDC.
