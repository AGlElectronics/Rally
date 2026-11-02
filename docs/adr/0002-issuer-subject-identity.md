# ADR 0002 — Identity key is `(issuer, subject)`

- **Status:** Accepted
- **Date:** 2026-11-02
- **Deciders:** Rally engineering
- **Related:** `architecture.md` §4.4

## Context

Rally must support both an initial standalone mode and later integration
with a shared OIDC identity provider (`architecture.md` §4). Email
addresses are mutable and can be shared across accounts; the OIDC `sub`
claim is stable and provider-assigned. The ecosystem spec forbids using
mutable attributes as the identity primary key.

## Decision

- `users.id` is a UUID assigned by Rally. It is the only identifier used
  by foreign keys inside Rally.
- A separate `external_identities` table stores `(user_id, issuer,
  subject)` triples with `UNIQUE (issuer, subject)`. Every authentication
  (OIDC or dev bypass) resolves to a row here first; the user row is only
  touched to refresh display name / email / `last_login_at`.
- The dev bypass (`AUTH_MODE=dev`) uses the reserved issuer `"dev"`. The
  subject is the X-Dev-User header value (default `dev-admin`). A dev
  account can therefore never collide with a real OIDC subject.
- Email is indexed case-insensitively for display-side lookups only. Email
  equality is **never** used to merge or match identities automatically.
- Linking an existing standalone user to a new OIDC identity is a
  deliberate workflow (future ADR + migration), not an implicit email
  match. The scaffold does not implement linking; it creates a new local
  user for each never-seen `(issuer, subject)` pair.

## Consequences

- **+** Users keep stable IDs across standalone→SSO migration; audit
  history, submissions, memberships keep working.
- **+** A compromised email or email-provider impersonation cannot be used
  to take over a Rally account.
- **−** Operators migrating a standalone deployment to shared SSO will
  need an explicit "link my accounts" flow; this is called out as a
  follow-up in the delivery plan.
