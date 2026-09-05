# homerun2-demo-pitcher

demo-pitcher

[![Build & Test](https://github.com/stuttgart-things/homerun2-demo-pitcher/actions/workflows/build-test.yaml/badge.svg)](https://github.com/stuttgart-things/homerun2-demo-pitcher/actions/workflows/build-test.yaml)

## API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | `GET` | None | Health check (returns version, commit, date) |
| `/pitch` | `POST` | Bearer token | Submit a message to Redis Streams |

<details>
<summary><b>Pitch a message</b></summary>

```bash
curl -X POST http://localhost:8080/pitch \
  -H "Authorization: Bearer <YOUR_AUTH_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Test Notification",
    "message": "Hello from homerun2-demo-pitcher",
    "severity": "info",
    "author": "test"
  }'
```

</details>

## Deployment

<details>
<summary><b>Container image (ko / ghcr.io)</b></summary>

```bash
docker pull ghcr.io/stuttgart-things/homerun2-demo-pitcher:<tag>

docker run \
  -e REDIS_ADDR=redis -e REDIS_PORT=6379 \
  -e REDIS_STREAM=homerun \
  -e AUTH_TOKEN=mysecret \
  -p 8080:8080 \
  ghcr.io/stuttgart-things/homerun2-demo-pitcher:<tag>
```

</details>

## Development

<details>
<summary><b>Configuration reference</b></summary>

| Variable | Description | Default |
|----------|-------------|---------|
| `DEMO_MODE` | `api` (auth on `/pitch`), `web` (UI, `/pitch` unauthenticated), `full` (UI + auth) | `api` |
| `PITCH_TARGET` | Backend: `redis`, `omni-pitcher`, `both` or `file` | `redis` |
| `OMNI_PITCHER_URL` | omni-pitcher endpoint, used by `omni-pitcher` and `both` | `http://localhost:4000` |
| `OMNI_PITCHER_API_PATH` | Path appended to that endpoint | `generic` |
| `PITCHER_FILE` | Output file, used by `file` | `pitched.log` |
| `REDIS_ADDR` | Redis server address | `localhost` |
| `REDIS_PORT` | Redis server port | `6379` |
| `REDIS_PASSWORD` | Redis password | (empty) |
| `REDIS_STREAM` | Redis stream name | `homerun` |
| `PORT` | HTTP server port | `8080` |
| `AUTH_TOKEN` | Bearer token. Required on `/pitch` in `api` and `full` mode; also sent as `Authorization: Bearer` when pitching to omni-pitcher | (empty) |
| `LOG_FORMAT` | `json` or `text` | `json` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

`PITCH_TARGET` is matched exactly. A value that is none of the four — `http`,
say — is **not** an error: it falls through to the `redis` default, the process
logs `pitcher backend: redis` and every pitch goes to `REDIS_STREAM` instead of
where you meant. Check the startup line before believing a chained setup works.

<details>
<summary><b>Scheduler (optional, off by default)</b></summary>

| Variable | Description | Default |
|----------|-------------|---------|
| `PITCH_ENABLED` | `true` starts the periodic pitcher | `false` |
| `PITCH_INTERVAL` | Go duration between bursts | `10s` |
| `PITCH_BURST_SIZE` | Messages per burst | `1` |
| `PITCH_PROFILE` | Profile name to send | `default` |
| `PITCH_PROFILE_DIR` | Directory the profiles are read from | `profiles` |

</details>

<details>
<summary><b>AI message generation (optional, off by default)</b></summary>

| Variable | Description | Default |
|----------|-------------|---------|
| `AI_ENABLED` | `true` generates message text via an LLM | `false` |
| `AI_PROVIDER` | Provider name | `ollama` |
| `AI_ENDPOINT` | Provider endpoint | `http://localhost:11434` |
| `AI_MODEL` | Model name | `llama3` |
| `AI_API_KEY` | API key, where the provider needs one | (empty) |

</details>

</details>

## Testing

```bash
# Unit tests (no Redis needed)
go test ./...

# Integration tests (Dagger + Redis)
task build-test-binary

# Lint
task lint

# Build + scan image
task build-scan-image-ko
```

## Links

- [Releases](https://github.com/stuttgart-things/homerun2-demo-pitcher/releases)
- [homerun-library](https://github.com/stuttgart-things/homerun-library)
