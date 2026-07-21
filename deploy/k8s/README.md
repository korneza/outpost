# Kubernetes manifest

A minimal `Deployment` + `Service` + `ConfigMap` for running `outpost serve` in a cluster.

**Not deployed anywhere** — this is deploy-ready config, not a running deployment. `image: ghcr.io/korneza/outpost:latest` won't resolve to anything real until a GitHub Release has actually been cut (goreleaser + the `dockers` block in `.goreleaser.yaml` build and push that image on release, which hasn't happened yet).

`state_db` at `/data/outpost.db` is backed by `emptyDir`, so pin state does **not** survive a pod restart in this minimal manifest — replace with a `PersistentVolumeClaim` before running this for real; a rug-pull re-detection after every restart is a real security regression, not a cosmetic one (see the pinning-restart-hydration fix in the decision log, a finding from the earlier security review).

Apply with:

```bash
kubectl apply -f outpost.yaml
```
