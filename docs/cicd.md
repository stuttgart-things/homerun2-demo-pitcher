# CI/CD

## GitHub Actions Workflows

### Core

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `build-test.yaml` | PR / push to main | Dagger lint + build + test |
| `build-scan-image.yaml` | PR / push to main | ko build + Trivy scan; PR job tags `pr-<num>-<sha>` for preview envs, main job tags `:main` |
| `release.yaml` | After image build / manual | Semantic release + stage image + push kustomize OCI |
| `pages.yaml` | After release / manual | Deploy MkDocs to GitHub Pages |
| `lint-repo.yaml` | PR / push to main | Repository linting |

### PR-preview env

These four together drive the per-PR ephemeral preview environment on `homerun2-dev` for PRs carrying the `preview` label.

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `build-scan-image.yaml` (PR job) | PR opened/updated | ko image tagged `pr-<num>-<sha>` + `pr-<num>` consumed by the per-PR ArgoCD Application |
| `push-kustomize-pr.yaml` | PR opened/updated | Kustomize OCI tagged `pr-<num>-<sha>` (renders `kcl/main.k` against `tests/kcl-deploy-profile.yaml`) |
| `comment-preview-url.yaml` | PR opened/reopened | Sticky bot comment with the preview URL, namespace, and ArgoCD link |
| `cleanup-pr-artifacts.yaml` | PR closed | Deletes both ghcr.io packages so version histories don't fill with PR debris |

See [Preview Environments](preview-environments.md) for the full flow, AppSet anatomy, and troubleshooting.

## Dagger Functions

The `dagger/` module provides:

| Function | Description |
|----------|-------------|
| `Lint` | Go linting via golangci-lint |
| `Build` | Build Go binary |
| `BuildImage` | Build container image with ko |
| `ScanImage` | Trivy vulnerability scan |
| `BuildAndTestBinary` | Build + Redis integration test |

## Taskfile

Common tasks available via `task`:

```bash
task lint                  # Run Go linter
task build-test-binary     # Build + test with Redis via Dagger
task build-output-binary   # Build Go binary
task build-scan-image-ko   # Build, push, scan container image
task render-manifests      # Render KCL manifests
task deploy-kcl            # Deploy to cluster
```

## Release Process

Releases are fully automated via [semantic-release](https://semantic-release.gitbook.io/):

- `fix:` commits trigger a **patch** bump
- `feat:` commits trigger a **minor** bump
- Each release publishes the container image and kustomize OCI artifact to `ghcr.io`
- GitHub Pages documentation is deployed after each release
