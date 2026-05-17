# Preview Environments

Every pull request opened against `main` can spin up an ephemeral, fully-deployed instance of demo-pitcher on the `homerun2-dev` Kubernetes cluster — co-tenanted with omni-pitcher, core-catcher (web mode), and redis-stack so reviewers can click "Pitch" in demo-pitcher's UI and watch the event flow through the full HTTP chain into core-catcher's dashboard. The environment lives for as long as the PR is open and tears down automatically on merge or close.

This page covers how to use it, what each PR gets, the components that make it work, and how to troubleshoot.

## Quick start

1. Open a PR against `main`.
2. Add the `preview` label: `gh pr edit <num> --add-label preview`.
3. Wait 5–10 minutes for the image build, the kustomize-OCI push, and Argo's PullRequest generator poll (every 600s).
4. The preview-bot leaves a sticky comment on the PR with the URL.

Closing or merging the PR tears the namespace down automatically.

## What you get per PR

Each preview lives in its own namespace: `homerun2-demo-pitcher-pr-<num>` on `homerun2-dev`. The namespace contains:

| Workload | Purpose |
|--|--|
| `homerun2-demo-pitcher` | The system under test (this PR's commit). UI exposed at `demo-pr-<num>.…`. |
| `homerun2-omni-pitcher` (co-tenanted, pinned `v1.11.1`) | Receives `POST /pitch` from demo-pitcher's UI, writes to the Redis stream. UI / metrics reachable at `omni-demo-pr-<num>.…`. |
| `homerun2-core-catcher` (co-tenanted, pinned `v0.13.0`, web mode) | Consumes the same Redis stream and renders the dashboard at `cc-demo-pr-<num>.…` so reviewers can see what landed. |
| `redis-stack` | The bus all three share; persistence disabled (ephemeral). |

End-to-end flow: open demo-pitcher's UI → click "Pitch" → demo POSTs to omni-pitcher's `/pitch` (with `Authorization: Bearer <vault.authToken>`) → omni writes to `redis-stack` → core-catcher consumes and surfaces it in its dashboard. UI-to-UI in one PR namespace, mirroring the production HTTP topology.

## Why omni-pitcher is co-tenanted (and not just RedisPitcher)

demo-pitcher ships both a `RedisPitcher` (writes straight to Redis Streams via `homerun-library.EnqueueMessageInRedisStreams`) and an `HTTPPitcher` (POSTs to a remote `/pitch` endpoint). On paper, `RedisPitcher` would let us drop omni-pitcher from the preview and shave a workload.

In practice, demo-pitcher's UI surfaces a "Target Pitcher URL" field rendered from the configured HTTP endpoint. With no in-cluster omni-pitcher, the chart would have to either set `PITCH_TARGET=redis` (silencing the UI label but hiding the production HTTP path entirely from reviewers) or leave the UI showing demo-pitcher's compiled-in fallback `http://localhost:4000` (which 404s when clicked).

Co-tenanting omni-pitcher keeps the production HTTP shape intact: the UI shows the in-cluster service URL, "Pitch" actually POSTs across the network, omni-pitcher's middleware enforces auth, and core-catcher consumes from the same stream production does. The 2026-05-17 rollout comment thread on [stuttgart-things/homerun2-omni-pitcher#116](https://github.com/stuttgart-things/homerun2-omni-pitcher/issues/116) has the full design history.

## Why the `preview` label gate

Without the label, every renovate / dependabot dep-bump PR would spawn a namespace. Two problems:

- Branches predating the build-pr workflow have no `pr-<num>-<sha>` image or kustomize artifacts published — half-empty namespaces with sync errors.
- Bots open dozens of PRs per week; the preview infrastructure isn't built for that scale.

Human-opened PRs opt in via the label. Bots don't apply it, so they're excluded by default. The Argo AppSet's PullRequest generator filters on `labels: [preview]`.

## The flow, end to end

```
git push (PR opens)
   ├─► comment-preview-url.yaml  ─►  sticky bot comment with URL
   ├─► build-scan-image.yaml     ─►  ko-built image at ghcr.io/.../homerun2-demo-pitcher:pr-<num>-<sha>
   ├─► push-kustomize-pr.yaml    ─►  kustomize OCI at ghcr.io/.../homerun2-demo-pitcher-kustomize:pr-<num>-<sha>
   └─► build-test.yaml + lint    ─►  CI gates

Argo PullRequest generator (poll every 600s)
   └─► detects PR with `preview` label
       └─► renders parent Application `homerun2-demo-pitcher-pr-<num>` in argocd ns
           └─► chart emits child Applications targeting `homerun2-demo-pitcher-pr-<num>` ns:
               demo-pitcher, omni-pitcher, core-catcher, redis-stack, httproute

Kyverno ClusterPolicies (auto-fire on namespace create)
   ├─► generate ResourceQuota + LimitRange
   └─► generate 6 ExternalSecrets → ESO materializes Secrets from Vault preview-env

PR close
   ├─► AppSet drops the entry → finalizer cascade prunes child Apps + workloads
   ├─► cleanup-pr-artifacts.yaml deletes both ghcr.io packages
   └─► Kyverno ClusterCleanupPolicy reaps any empty namespace shell left behind
```

## The four PR-preview workflows in this repo

All four are in `.github/workflows/` and trigger on `pull_request` events targeting `main`.

| Workflow | Trigger | Output |
|--|--|--|
| `build-scan-image.yaml` | PR opened/updated | ko-built image tagged `pr-<num>-<sha>` + `pr-<num>` |
| `push-kustomize-pr.yaml` | PR opened/updated | kustomize OCI tagged `pr-<num>-<sha>` (renders `kcl/main.k` against `tests/kcl-deploy-profile.yaml`) |
| `comment-preview-url.yaml` | PR opened/reopened | Sticky comment with URL, namespace, ArgoCD link |
| `cleanup-pr-artifacts.yaml` | PR closed | Deletes both ghcr.io packages so version histories don't fill with PR debris |

All four delegate to reusable workflows in `stuttgart-things/github-workflow-templates`.

## The Argo AppSet

Lives at `stuttgart-things/argocd` under `platforms/homerun2-pr-preview/appset-demo-pitcher-pr-preview.yaml`. The shape (abridged):

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: homerun2-demo-pitcher-pr-preview
  namespace: argocd
spec:
  generators:
    - matrix:
        generators:
          - clusters: { selector: { matchLabels: { homerun2-pr-preview: "true" } } }
          - pullRequest:
              github:
                owner: stuttgart-things
                repo: homerun2-demo-pitcher
                tokenRef: { secretName: homerun2-omni-pitcher-pat, key: token }
                labels: [preview]               # the gate
              requeueAfterSeconds: 600          # poll cadence
  template:
    metadata:
      name: 'homerun2-demo-pitcher-pr-{{ .number }}'
      finalizers: [resources-finalizer.argocd.argoproj.io]
    spec:
      source:
        repoURL: https://github.com/stuttgart-things/argocd.git
        path: apps/homerun2/install
        helm:
          valuesObject:
            destination:
              name: '{{ .name }}'
              namespace: 'homerun2-demo-pitcher-pr-{{ .number }}'
            demoPitcher:
              enabled: true
              version: 'pr-{{ .number }}-{{ .head_sha }}'
              hostname: 'demo-pr-{{ .number }}.homerun2-dev.sthings-vsphere.labul.sva.de'
              inlineHttpRoute: true
              # Chart injects PITCH_TARGET=http + OMNI_PITCHER_URL=<this> +
              # OMNI_PITCHER_API_PATH=pitch + AUTH_TOKEN (secretKeyRef) into
              # the Deployment env when set. See "The env-injection chain" below.
              omniPitcherUrl: 'http://homerun2-omni-pitcher.homerun2-demo-pitcher-pr-{{ .number }}.svc.cluster.local'
            omniPitcher:
              enabled: true
              version: v1.11.1
              hostname: 'omni-demo-pr-{{ .number }}.homerun2-dev.sthings-vsphere.labul.sva.de'
              inlineHttpRoute: true
            coreCatcher:
              enabled: true
              version: v0.13.0
              kustomizeVersion: v0.13.0
              hostname: 'cc-demo-pr-{{ .number }}.homerun2-dev.sthings-vsphere.labul.sva.de'
              inlineHttpRoute: true
            redisStack:
              enabled: true
              persistence: { enabled: false }
              auth: { existingSecret: redis-stack-auth }
            # all other components off
            httpRoute:
              enabled: true
              gateway: { name: homerun2-dev-gateway, namespace: default }
      syncPolicy:
        automated: { prune: true, selfHeal: true }
        syncOptions: [CreateNamespace=true, ServerSideApply=true]
```

The AppSet renders one **parent** Argo `Application` per labelled PR. The parent's source is the `apps/homerun2/install` chart in the `stuttgart-things/argocd` catalog. The chart emits **child** Applications (one per enabled component: demo-pitcher, omni-pitcher, core-catcher, redis-stack) on the target cluster.

## The env-injection chain

The chart's `apps/homerun2/install/templates/demo-pitcher.yaml` template, when `demoPitcher.omniPitcherUrl` is set, patches demo-pitcher's Deployment to add four env vars on top of what `kcl/main.k` renders:

| Env var | Value | Why |
|--|--|--|
| `PITCH_TARGET` | `http` | Switches demo-pitcher's `/pitch` handler from its baked-in `RedisPitcher` default to `HTTPPitcher` |
| `OMNI_PITCHER_URL` | `http://homerun2-omni-pitcher.<ns>.svc.cluster.local` | In-cluster Service URL of the co-tenanted omni-pitcher |
| `OMNI_PITCHER_API_PATH` | `pitch` | Without this override, demo-pitcher's default `generic` produces `/generic` which 404s on omni-pitcher (whose generic route is `/pitch`) |
| `AUTH_TOKEN` | `secretKeyRef → homerun2-demo-pitcher-token / auth-token` | Sent as `Authorization: Bearer …`. Same Vault property (`preview-env.authToken`) populates both demo-pitcher's and omni-pitcher's `-token` Secrets via ESO, so the bearer-token compare in omni-pitcher's `authMiddleware` matches. |

The patches use the existing `homerun2.redisAddrPatch` helper's `extraEnv` plumbing; strategic-merge merges container env by `name`, so `PITCH_TARGET=http` overrides the KCL-baked default `redis`.

## The policy generators

Lives alongside the AppSet at `stuttgart-things/argocd` under `platforms/homerun2-pr-preview/appset-policies.yaml`. A matrix-generator fans out across (preview-enabled clusters) × (policy types) per component:

| Policy | What it does |
|--|--|
| `homerun2-demo-pitcher-preview-quota` | Kyverno `ClusterPolicy` → generates `ResourceQuota` + `LimitRange` in each PR namespace |
| `homerun2-demo-pitcher-preview-secrets` | Kyverno `ClusterPolicy` → generates 6 `ExternalSecret`s; ESO pulls from Vault `homerun2-pr/data/preview-env` |
| `homerun2-demo-pitcher-preview-sweep` | Kyverno `ClusterCleanupPolicy` → cron-reaps empty PR namespace shells |

The 6 `ExternalSecret`s ESO materializes: `redis-stack-auth`, `homerun2-demo-pitcher-redis`, `homerun2-demo-pitcher-token`, `homerun2-omni-pitcher-redis`, `homerun2-omni-pitcher-token`, `homerun2-core-catcher-redis`. The two `-token` Secrets share the same Vault property (`authToken`) so demo's bearer token matches what omni-pitcher expects.

**No seed-data policy for demo-pitcher's previews.** The preview's value is "human clicks the UI to see events flow"; a pre-populated fixture would muddy that — reviewers couldn't tell their click from the seed batch. Decision: [stuttgart-things/homerun2-omni-pitcher#116](https://github.com/stuttgart-things/homerun2-omni-pitcher/issues/116) (2026-05-17 corrected-design comment).

## HTTPRoute: Option B (inline in the kustomize OCI)

The HTTPRoute exposing demo-pitcher externally is rendered by `kcl/httproute.k` and ships **inside the kustomize OCI**, alongside the Service. They land in the same kustomize apply, eliminating the cross-Application race that previously let Cilium's gateway controller stamp a sticky `BackendNotFound` (tracked under [stuttgart-things/argocd#116](https://github.com/stuttgart-things/argocd/issues/116)). Three places have to agree:

| Repo | Setting |
|--|--|
| `homerun2-demo-pitcher` (this repo) | `tests/kcl-deploy-profile.yaml` → `config.httpRouteEnabled: true` |
| `stuttgart-things/argocd` | `apps/homerun2/install` → `demoPitcher.inlineHttpRoute` flag patches the rendered HTTPRoute's parentRef + hostname per env, and excludes demo-pitcher from the standalone httproute Application |
| `stuttgart-things/argocd` (AppSet) | Set `demoPitcher.inlineHttpRoute: true` in `valuesObject` (already in the AppSet template above) |

The same applies to the co-tenanted `omniPitcher.inlineHttpRoute: true` and `coreCatcher.inlineHttpRoute: true` — all three components ship their HTTPRoute inline in this preview, so three distinct external hostnames per namespace.

With all three set, `HTTPRoute/homerun2-demo-pitcher` lands `ResolvedRefs: True` on first reconcile. No manual `kubectl annotate httproute reconcile-bump=$(date +%s) --overwrite` required.

## Lifecycle

| Event | Result |
|--|--|
| PR opened with `preview` label | Sticky bot comment posted; CI builds image + kustomize OCI; AppSet picks it up within 600s; namespace + workloads spin up |
| PR updated (new commit) | Image + kustomize OCI rebuilt with new `<sha>`; AppSet detects the head-SHA change; rolling update of Deployments |
| PR `preview` label removed | AppSet drops the entry; finalizer prune cascades teardown |
| PR closed (merged or rejected) | AppSet drops the entry → teardown; `cleanup-pr-artifacts.yaml` deletes ghcr.io packages |

The `resources-finalizer.argocd.argoproj.io` finalizer on the parent Application is critical — without it, Argo would delete the parent instantly when the AppSet drops it, orphaning child Apps + workload pods. With it, Argo runs prune on every managed resource first.

## Troubleshooting

| Symptom | Likely cause | Fix |
|--|--|--|
| No bot comment, no namespace | `preview` label missing | `gh pr edit <num> --add-label preview` |
| Bot comment present, namespace never appears | AppSet hasn't polled yet | Wait up to 10 min, or `kubectl -n argocd annotate appset homerun2-demo-pitcher-pr-preview argocd.argoproj.io/refresh=hard` |
| Parent Application sync error: `failed to load: oci pull` | Image / kustomize OCI build still running or failed | Check the PR's Actions tab — `build-pr` and `push-kustomize` must both be green |
| Pods stuck `ImagePullBackOff` | ghcr.io tag not yet pushed (CI still running) or PR closed (cleanup workflow already ran) | Wait for build / reopen the PR |
| Pods CrashLoopBackOff with `WRONGPASS` | ESO hasn't materialized `redis-stack-auth` Secret yet | Check `kubectl -n homerun2-demo-pitcher-pr-<num> get externalsecret`; refresh if not Ready |
| HTTPRoute `ResolvedRefs: False` | Service didn't land before HTTPRoute (pre-Option-B environments only) | Should not happen now; if it does: `kubectl annotate httproute <name> reconcile-bump=$(date +%s) --overwrite -n homerun2-demo-pitcher-pr-<num>` and file an issue |
| UI "Pitch" returns 404 | demo-pitcher hitting a wrong path on omni-pitcher; `OMNI_PITCHER_API_PATH` not injected | Check `kubectl -n homerun2-demo-pitcher-pr-<num> exec deploy/homerun2-demo-pitcher -- env \| grep OMNI_PITCHER_API_PATH` — should be `pitch`, not `generic` |
| UI "Pitch" returns 401 | `AUTH_TOKEN` missing on demo-pitcher's Deployment or `homerun2-demo-pitcher-token` Secret empty | Check the ExternalSecret status and the Deployment env |
| Dashboard loads but is empty | core-catcher pod healthy but no events sent yet | Open demo-pitcher's UI and click "Pitch" |
| Namespace stuck Terminating after PR close | Finalizer on a CRD instance | `kubectl get all,externalsecret -n homerun2-demo-pitcher-pr-<num>` to find the blocker |

## See also

- [stuttgart-things/argocd `apps/homerun2`](https://github.com/stuttgart-things/argocd/tree/main/apps/homerun2) — the install chart + Kyverno policy charts the AppSet consumes
- [stuttgart-things/argocd `platforms/homerun2-pr-preview`](https://github.com/stuttgart-things/argocd/tree/main/platforms/homerun2-pr-preview) — the AppSets + policies for all components
- [stuttgart-things/homerun2-omni-pitcher#116](https://github.com/stuttgart-things/homerun2-omni-pitcher/issues/116) — the umbrella rollout issue tracking all 8 components
- [stuttgart-things/argocd#116](https://github.com/stuttgart-things/argocd/issues/116) — the HTTPRoute creation-order race writeup that motivated Option B
- [stuttgart-things/github-workflow-templates](https://github.com/stuttgart-things/github-workflow-templates) — the four reusable PR-preview workflows this repo delegates to
