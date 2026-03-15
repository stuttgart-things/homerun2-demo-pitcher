# KCL Deployment — homerun2-demo-pitcher

KCL module for rendering and deploying homerun2-demo-pitcher Kubernetes manifests.

## Resources

| Resource | Name | Description |
|---|---|---|
| Namespace | `homerun2` | Target namespace |
| ServiceAccount | `homerun2-demo-pitcher` | Pod identity |
| ConfigMap | `homerun2-demo-pitcher-config` | Non-sensitive config (`LOG_LEVEL`) |
| Secret | `homerun2-demo-pitcher-redis` | Redis password (only if `redisPassword` is set) |
| Deployment | `homerun2-demo-pitcher` | Application workload |
| Service | `homerun2-demo-pitcher` | ClusterIP service (port 80 → 8080) |
| HTTPRoute | `homerun2-demo-pitcher` | Gateway API ingress (only if `httpRouteEnabled=true`) |

## Prerequisites

- [KCL CLI](https://kcl-lang.io/) v0.11+
- `kubectl` with a valid `KUBECONFIG`
- A running Redis instance reachable from the target namespace

## Render manifests

```bash
kcl run .
```

## Deploy with defaults

```bash
kcl run . | python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
for m in data.get('manifests', []):
    if m: print('---'); print(yaml.dump(m))
" | kubectl apply -f -
```

## Deploy with overrides

Use `-D` flags to override schema defaults defined in `schema.k`:

```bash
kcl run . \
  -D config.image=ghcr.io/stuttgart-things/homerun2-demo-pitcher:v1.4.0 \
  -D config.redisAddr=redis-stack.homerun2-flux.svc.cluster.local \
  -D config.redisPassword=changeme \
  -D config.httpRouteEnabled=true \
  -D config.httpRouteParentRefName=movie-scripts2-gateway \
  -D config.httpRouteParentRefNamespace=default \
  -D config.httpRouteHostname=homerun2-demo-pitcher.movie-scripts2.sthings-vsphere.labul.sva.de \
| python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
for m in data.get('manifests', []):
    if m: print('---'); print(yaml.dump(m))
" | kubectl apply -f -
```

## Configuration reference

| Parameter | Default | Description |
|---|---|---|
| `config.name` | `homerun2-demo-pitcher` | Resource name |
| `config.namespace` | `homerun2` | Target namespace |
| `config.image` | `ghcr.io/stuttgart-things/homerun2-demo-pitcher:latest` | Container image |
| `config.replicas` | `1` | Pod replicas |
| `config.redisAddr` | `redis-stack.homerun2.svc.cluster.local` | Redis host |
| `config.redisPort` | `6379` | Redis port |
| `config.redisStream` | `homerun` | Redis stream name |
| `config.redisPassword` | _(empty)_ | Redis password (creates Secret if set) |
| `config.demoMode` | `web` | Demo mode (`web`) |
| `config.pitchTarget` | `redis` | Pitch target backend |
| `config.servicePort` | `80` | Service port |
| `config.containerPort` | `8080` | Container port |
| `config.httpRouteEnabled` | `false` | Enable Gateway API HTTPRoute |
| `config.httpRouteParentRefName` | _(empty)_ | Gateway name |
| `config.httpRouteParentRefNamespace` | _(empty)_ | Gateway namespace |
| `config.httpRouteHostname` | _(empty)_ | Ingress hostname |
| `config.extraEnvVars` | `{}` | Additional env vars as key-value map |

## Example: deploy to movie-scripts cluster

```bash
export KUBECONFIG=/home/sthings/.kube/movie-scripts

kcl run . \
  -D config.image=ghcr.io/stuttgart-things/homerun2-demo-pitcher:v1.4.0 \
  -D config.redisAddr=redis-stack.homerun2-flux.svc.cluster.local \
  -D config.httpRouteEnabled=true \
  -D config.httpRouteParentRefName=movie-scripts2-gateway \
  -D config.httpRouteParentRefNamespace=default \
  -D config.httpRouteHostname=homerun2-demo-pitcher.movie-scripts2.sthings-vsphere.labul.sva.de \
| python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
for m in data.get('manifests', []):
    if m: print('---'); print(yaml.dump(m))
" | kubectl apply -f -
```

> **Note:** The Redis password should be managed separately via a Kubernetes Secret
> (`homerun2-demo-pitcher-redis`) rather than passed as a `-D` flag in production.
