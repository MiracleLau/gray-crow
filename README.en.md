<p align="center">
  <img src="web/public/logo.png" width="112" alt="GrayCrow">
</p>

<h1 align="center">GrayCrow</h1>

<p align="center">Sensitive data discovery for public GitHub code</p>

<p align="center">
  <a href="README.md">中文</a> · English
</p>

## Overview

GrayCrow periodically searches public code through the GitHub Code Search API. When a configured domain, email address or project name appears next to a password, key or token, the match is re-checked locally with regex and entropy rules, deduplicated, and the hits from one scan are delivered to the notification channels bound to the rule (Feishu, WeCom, DingTalk, generic webhook or SMTP). Recipients sign in to the console to mark findings as false positive, handled or ignored, or to allowlist the repository.

GrayCrow ships as a single binary with the frontend embedded via `go:embed`. SQLite is the default database; MySQL and PostgreSQL are supported as well.

## Scope

- Only indexed public repositories are searched, and only their default branch. Indexing usually lags behind by minutes to hours.
- A single query returns at most 1000 results. Broad keywords are split into date windows, which can still miss content.
- Code Search has its own rate limit of 10 requests per minute when authenticated. The 5000 requests per hour limit of other REST endpoints does not apply here.
- Only matched snippets are stored. Files are never saved in full and repositories are never cloned.
- Multiple tokens belonging to the same GitHub account share one quota. Expanding capacity requires separate accounts; extra tokens provide failover and load spreading.

## Quick start

Requires Go 1.26+ and Bun 1.x. Building the frontend also requires Node.js 20+, since `vue-tsc` does not run correctly on the Bun runtime.

```bash
cp .env.example .env             # set MASTER_KEY, at least 32 characters
cd web && bun install && cd ..   # first run only
make build-web                   # first run only, go:embed needs web/dist
make dev                         # API on :8080, web on :5173
```

The first start creates the schema and an administrator account `admin / admin123`. Change the password after signing in. For frontend-only work, `make dev-mock` serves every API from a local mock.

| Command | Description |
| --- | --- |
| `make dev` / `make dev-mock` | Full stack / mock-only frontend |
| `make dev-api` / `make dev-web` | Backend only / frontend only |
| `make test` | `go test ./...` |
| `make lint` | `go vet`, i18n key check, frontend build |
| `make build` | Build the frontend and compile `bin/graycrow` |
| `make smoke` | SQLite smoke test |
| `make clean` | Remove `bin/` and `web/dist/` |

## Deployment

```bash
make build
./bin/graycrow
```

`make build` embeds the frontend into `bin/graycrow`, which can be copied to a server and run directly. Switch databases through `DB_DRIVER` and `DB_DSN` only; SQLite needs no external service.

Set `COOKIE_SECURE=true` when running behind a reverse proxy over HTTPS. The frontend can also be served by an external static server, in which case point `STATIC_DIR` at the build output.

Docker:

```bash
cd deploy
MASTER_KEY=$(openssl rand -base64 32) docker compose --profile sqlite up -d
```

`deploy/docker-compose.yml` defines the `sqlite`, `mysql` and `postgres` profiles. Enable one at a time.

## Local demo

`cmd/fakegithub` emulates GitHub Code Search with fixed results and rate limit headers, so the whole flow (create a rule, scan, match, notify, handle) can be exercised without a real token:

```bash
make fake-gh                                             # terminal A
GITHUB_API_BASE=http://127.0.0.1:18081 make dev           # terminal B
```

Smoke test across databases:

```bash
make smoke                                # SQLite
scripts/smoke.sh mysql '<dsn>'
scripts/smoke.sh postgres '<dsn>'
```

## Configuration

Precedence, highest first: environment variables > `.env` > `config.yaml` > built-in defaults.

Start from `.env.example` or `config.example.yaml`. `config.yaml` may contain `master_key`, so it is excluded by `.gitignore`. Keys use the same names as the environment variables and are written flat; DSNs need no shell escaping. Unknown keys produce a warning at startup, and changes take effect after a restart.

## Notifications

New hits from one scan are combined into a single message containing the total count, the repository distribution and up to 20 details; remaining entries are reported as a count. The same leak hit again only increments its occurrence count and is not re-notified. Every send creates a notification record with channel, count, status, retry count and body. A missed notification can be resent from the alert detail page.

## Operations

Key rotation: put the new value in `MASTER_KEY` and the old one in `MASTER_KEY_OLD`, start once, and remove the old value after the log line `已用当前 MASTER_KEY 重加密 N 个字段` appears. Ciphertexts carry a key fingerprint, so a skipped rotation step does not make data unreadable; the worst case is a "key not found" error.

Two-factor authentication: avatar menu → account security. Authenticator apps are supported and ten recovery codes are issued. Administrators can reset it from the user management page.

Dry-run diagnosis: the rule list offers a diagnosis based on the last 30 days of hits and false positives, with one-click adoption of its keyword suggestions.

Retention: scan logs are kept for 30 days and audit logs for 180 days. Alerts and notification records are never removed automatically. Back up the `data/` directory.

Interface language: switch between Chinese and English in the top right corner. The choice is stored in the browser.

## Layout

```
cmd/graycrow/         entry point, HTTP server
cmd/fakegithub/       local fake GitHub for demos
internal/config/      environment and config file loading
internal/server/      Gin engine, routes, static assets
internal/api/         HTTP handlers
internal/middleware/  session, RBAC, CSRF, access log redaction, recovery, request id
internal/model/       GORM models
internal/store/       schema creation and migration
internal/service/     rules, alerts, channels, key pool, audit, retention
internal/engine/      scheduler loop: due rules → scan → alerts → summary notification
internal/scanner/     query building, GitHub client, second-pass detection
internal/schedule/    next-run calculation for interval and cron
internal/notify/      Feishu, WeCom, DingTalk, generic webhook, SMTP
internal/crypto/      AES-GCM field encryption and key rotation
internal/httpx/       response envelope and error codes
web/                  Bun + Vite + Vue3 + Naive UI
docs/                 technical documentation
deploy/               Dockerfile, docker-compose
scripts/              multi-database smoke scripts
```

## Documentation

| Document | Contents |
| --- | --- |
| [01 Architecture](docs/01-architecture.md) | Layers, scan pipeline, technology choices, environment variables, deployment |
| [02 Data model](docs/02-data-model.md) | Tables, indexes, cross-database compatibility, migrations |
| [03 API](docs/03-api.md) | Response format, error codes, complete route table |
| [04 Scanner](docs/04-scanner.md) | Query building, key pool scheduling, second-pass detection, deduplication, alert state machine |
| [05 Frontend](docs/05-frontend.md) | Routes and pages, design tokens, state management, requests and mocks |
| [06 Roadmap](docs/06-roadmap.md) | Phases and acceptance criteria |

## Notice

Use GrayCrow only against assets you are authorized to inspect, and comply with the GitHub Terms of Service and applicable law.
