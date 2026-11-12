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
- Rally uses the shared Con Services packages `denlink`, `groupmatch` and
  `authbase` (generated copies from `con-services` `shared/go`, checked by
  CI's *Shared code drift* job).
- `GET /api/role-catalog` follows the 1.0 role catalog contract: it needs
  HR's service token (`HR_SERVICE_TOKEN`, new, a secret) or a staff
  sign-in, lists each role once with `group_display`, `privileged`,
  `dynamic` and `default`, and has a `revision` that is also its `ETag`
  (`If-None-Match` gives `304`). HR can read it only once it sends the
  token. A role can override its `privileged` flag in `access-config.toml`.
- `GET /api/den/state` and the Den connection status report `revoked` when
  Den rejected Rally's credential; admins then see *Den rejected this
  module's credential: pair again* and can paste a new pairing URL. The
  migration `20261111000001_den_link_revoked` adds `den_link.revoked_at`.
- Staff group matching follows the shared rule and can be set with
  `OIDC_GROUPS_CLAIM` (default `groups`), `OIDC_GROUP_MATCH` (`full` or
  `last`) and `OIDC_GROUP_PREFIX` (for example `/con2026`).
- `ATTENDEE_OIDC_AUDIENCE` (default `events-public-api`): the audience the
  submission site's access tokens must carry. It must differ from
  `OIDC_AUDIENCE`.
- Rate limits on everything under `/api/public/v1` (`PUBLIC_RATE_LIMIT`,
  default `120/m` per client IP, burst `PUBLIC_RATE_BURST` `30`) and on
  creating and submitting submissions, inviting co-presenters, invite codes
  and starting uploads (`SUBMISSION_RATE_LIMIT`, default `20/m` per person,
  burst `SUBMISSION_RATE_BURST` `10`). Over the limit the API answers
  `429 rate_limited` with `Retry-After`. `off` switches a limiter off.
- Kubernetes: the chart lists `DEN_LINK_ENCRYPTION_KEY` (required) and
  `HR_SERVICE_TOKEN` (optional) as keys of `rally-secrets`, sets
  `OIDC_AUDIENCE: events-api` and `ATTENDEE_OIDC_AUDIENCE: events-public-api`,
  mounts each web app's security headers, and has the new value
  `frontend.cspConnectSrc` for extra origins in the Content Security Policy.
- The web app images read `KEYCLOAK_ORIGIN` and `S3_ORIGIN` at start-up and
  put them into their Content Security Policy.

### Changed

- The Helm chart's image tags default to `1.0.0-1`, including the submission image.
- Den's backend is admitted by `networkPolicy.den` (on by default) for the role catalog and portal to-dos.
- `.env.example` lists `TELEGRAM_BOT_TOKEN` (empty), `TELEGRAM_API_URL`, `DEN_ALLOWED_HOSTS` and the Den link encryption keys.
- CI passes `den-major: "1"`. A release whose MAJOR is not 1 is refused once the shared `ci-v1` tag points at the workflow that has that input.
- Standalone Compose uses PostgreSQL `postgres:16.15-alpine`.
- The Helm chart description says CI renders and lints the chart; the first production install is still pending.
- The shared Den client is the current copy from `con-services`. Go error
  text is lowercase; the sentence an admin sees when pairing fails, and
  the text stored in `den_link.last_error`, stays the original wording.
  Rally does not send mail, so it does not fetch Den's SMTP password.
- `frontend.cspConnectSrc` is added only to `connect-src`. It used to be
  copied into `frame-src` as well. Put a sign-in server on
  `KEYCLOAK_ORIGIN` (the chart takes it from the app's issuer URL).
- `AUTH_MODE` now defaults to `oidc` and `APP_ENV` to `production`.
  `AUTH_MODE=dev` stops the backend unless `APP_ENV=development`; the
  development Compose file and `.env.example` set `APP_ENV=development`.
- The web apps send the **access token** instead of the ID token, and the
  backend checks its audience: `OIDC_AUDIENCE` now defaults to `events-api`
  (was the client id). Keycloak must add `events-api` to `events-spa`'s and
  `events-public-api` to `rally-submit`'s access tokens with an audience
  mapper; see [Set up Rally](https://aglelectronics.github.io/con-services/modules/rally/setup/#2-prepare-keycloak).
  Without it every API call answers `401 invalid token`.
- The web apps keep tokens in memory only and sign in again silently after
  a reload; tabs follow the Keycloak session instead of each other.
- The `roles` claim no longer grants staff roles: only the claim named by
  `OIDC_GROUPS_CLAIM` counts. Put people into groups, not realm roles.
- JSON request bodies are limited to 1 MiB (`413 request_too_large`), and
  error messages for broken JSON are clearer (`unknown field "…"`).
  `accept` and `reject` now refuse a broken body instead of ignoring it.
- The role catalog's `scope` is `organization` instead of
  `organization:<slug>`, and its `note` says *Rally* instead of *Events*.
- `DEN_LINK_ENCRYPTION_KEY` is required outside `APP_ENV=development`
  (a secret, 32 random bytes in base64); `DEN_ALLOWED_HOSTS` limits pasted
  pairing URLs when `DEN_INTERNAL_URL` is empty.

### Security

- A reload can sign in again on the staff app and the submission site.
  `/silent-callback.html` may be framed by that app itself
  (`frame-ancestors 'self'`, `X-Frame-Options: SAMEORIGIN`). Every other
  page stays unframeable. The Content Security Policy is the current
  shared one: images may also load from `S3_ORIGIN`, and
  `EXTRA_FRAME_SRC` stays empty. The submission site still takes the
  attendee realm's server as `KEYCLOAK_ORIGIN` (audience
  `events-public-api` is not an origin) and the upload bucket as
  `S3_ORIGIN`.
- The Den credential is stored encrypted with `DEN_LINK_ENCRYPTION_KEY`
  (AES-256-GCM); a credential stored before is encrypted on its next read.
  Rotation: `DEN_LINK_ENCRYPTION_KEY_PREVIOUS`.
- Staff and attendee realms are kept apart more strictly: issuers are
  compared without regard to case, default port or trailing slash, the
  backend refuses to start when both realms' discovery documents name the
  same issuer, and a token from the other realm is refused with `401`
  before any other check. ID tokens are refused on every API.
- API responses carry `X-Content-Type-Options`, `X-Frame-Options`,
  `Referrer-Policy` and a `Content-Security-Policy`; the web apps' nginx
  adds the shared Con Services headers (CSP limited to the app, the sign-in
  server and the upload bucket). The inline theme and silent-sign-in
  scripts moved to files so scripts can be limited to the app's origin.
- The `X-Dev-User` and `X-Dev-Groups` headers are allowed through CORS only
  in dev mode and deleted from every request otherwise.
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
