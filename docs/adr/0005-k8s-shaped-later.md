# ADR 0005 — K8s-shaped manifests now, production chart later

- **Status:** Accepted
- **Date:** 2026-11-02
- **Deciders:** Rally engineering
- **Related:** `architecture.md` §3.3; `techstack.md` §8

## Context

The ecosystem spec requires every application to be
"Kubernetes-compatible" from day one, but explicitly defers Helm, GitOps
manifests, and automated DNS/TLS until a specific deployment task calls
for them:

> Helm charts, GitOps manifests, automated DNS/TLS, and central-manager-
> driven installation are DEFERRED unless a module-specific deployment
> task requires them. — `techstack.md` §8

We still need to prove the container shape is K8s-safe (stateless API,
non-root, probe split, migration Job separation) before claiming the box
is ticked.

## Decision

- `deploy/kubernetes/` ships Deployment + Service manifests for the
  backend and frontend, a `Namespace`, and a one-shot `Job` for
  migrations. They are documented as **shapes**, not a production chart:
  images reference `latest` placeholders, resource requests are modest
  defaults, and no Ingress / certificate / HPA is included.
- Secrets (`rally-postgres`, `rally-s3`, `rally-oidc`) are referenced by
  name but not provided. Operators own secret distribution (sealed-secrets,
  SOPS, External Secrets, platform-native).
- A proper Helm chart or Kustomize overlay is a separate task tracked
  under the Phase 5 deliverables of `delivery-plan.md`.

## Consequences

- **+** We can show that migrations, probes, and non-root containers work
  by the time a real cluster rollout is scoped.
- **+** No premature coupling to a specific chart tool (Helm vs Kustomize
  vs raw manifests) — the stubs document invariants, not conventions.
- **−** Operators cannot `kubectl apply -R` and get a working stack. The
  README is explicit about this.
