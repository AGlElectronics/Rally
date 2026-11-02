# Rally — Kubernetes stubs

> **Status:** Scaffolded shape only. These manifests document the intended
> deployment topology; they are **not** a working production chart. Expect to
> adapt names, image registries, resource requests, probes and secret
> management for your cluster. See
> [`../../docs/adr/0005-k8s-shaped-later.md`](../../docs/adr/0005-k8s-shaped-later.md).

The runtime behaviour the manifests enforce today:

1. **Migrations run exactly once per rollout.** `migrate-job.yaml` is a
   one-shot `Job`; the API `Deployment` must not start before it reports
   `Succeeded`. GitOps/Argo users: set `argocd.argoproj.io/hook: PreSync` on
   the Job. See [`../../docs/adr/0003-migrate-job.md`](../../docs/adr/0003-migrate-job.md).

2. **Liveness vs readiness are different probes.** `/livez` returns 200 as
   long as the process is alive; `/readyz` ping-tests the DB and returns 503
   while it is unreachable. The frontend `Deployment` uses the same split.

3. **No MinIO in production.** The compose stack runs MinIO for local dev
   only. For K8s, point `S3_*` env at a managed S3-compatible bucket (AWS
   S3, GCS S3 interop, Backblaze B2, etc.) via a `Secret`. See
   [`../../docs/adr/0004-minio-never-production.md`](../../docs/adr/0004-minio-never-production.md).

4. **Secrets never baked in images.** The manifests reference `Secret`
   resources (`rally-postgres`, `rally-s3`, `rally-oidc`) that you create
   out-of-band with sealed-secrets, SOPS, External Secrets, or your
   platform's equivalent.

## Example apply order

```sh
kubectl apply -f namespace.yaml
# then whatever Secret tooling you use creates rally-postgres / rally-s3 / rally-oidc
kubectl apply -f migrate-job.yaml      # waits for completion
kubectl apply -f backend-deployment.yaml
kubectl apply -f frontend-deployment.yaml
kubectl apply -f ingress.yaml          # not included yet — host-specific
```
