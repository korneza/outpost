# Kubernetes manifest

A `Deployment` + `Service` + `ConfigMap` + `PersistentVolumeClaim` for running `outpost serve` in a cluster.

**Not deployed anywhere** — this is deploy-ready config, not a running deployment. `image: ghcr.io/korneza/outpost:v0.9.0-alpha` matches the `v0.9.0-alpha` release tag (goreleaser's `dockers` block in `.goreleaser.yaml` builds and pushes this on tag push) — update it to match whatever you actually deploy.

`state_db` at `/data/outpost.db` is backed by a `PersistentVolumeClaim` (1Gi, `ReadWriteOnce`), not `emptyDir` — pin state must survive a pod restart, or a previously-blocked drifted tool silently becomes callable again until the next `tools/list` re-detects it (see the pinning-restart-hydration fix in the decision log, a finding from the earlier security review). Don't swap this back to `emptyDir` for convenience.

The container runs with `runAsNonRoot`, `allowPrivilegeEscalation: false`, a read-only root filesystem, and all Linux capabilities dropped — the Docker image is already distroless/nonroot, this enforces the same posture at the k8s level so it can't be silently bypassed by a different base image later.

Apply with:

```bash
kubectl apply -f outpost.yaml
```
