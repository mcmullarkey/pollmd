---
title: Render
weight: 30
---

# Deploy on Render

Render supports deploying pollmd from a Dockerfile. This guide walks
through the one-time setup and daily operations.

## Prerequisites

- A [Render account](https://render.com)
- Your pollmd repo pushed to GitHub / GitLab

## One-time setup

1. In the Render Dashboard, click **+ New** → **Blueprint**.
2. Connect your pollmd GitHub repo.
3. Render auto-detects `deploy/render/render.yaml` and creates:
   - A **Web Service** (Docker) that runs the Go binary
   - A **Persistent Disk** at `/var/db/survey` for the DuckDB file

The `render.yaml` in the repo configures:

| Setting | Value |
|---------|-------|
| Health check path | `/healthz` |
| Health check timeout | 60 seconds |
| Disk mount | `/var/db/survey` |
| Admin token | Auto-generated, set via `SURVEY_ADMIN_TOKEN` |

## Creating a poll

Once the service is up, create a poll via the admin API:

```bash
curl -X POST https://<your-service>.onrender.com/admin/surveys \
  -H "Authorization: Bearer <SURVEY_ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"survey_id":"dinner","answers":["pizza","tacos","sushi"]}'
```

Then share the vote links:

```
https://<your-service>.onrender.com/dinner/pizza
https://<your-service>.onrender.com/dinner/tacos
https://<your-service>.onrender.com/dinner/sushi
```

## Viewing results

```
https://<your-service>.onrender.com/result/dinner
```

## Quack note

Render does not expose a TCP proxy, so the Quack admin channel
(Railway feature) is unavailable on Render. Use the HTTP admin
endpoints (`/admin/*`) for poll management instead.

## Env vars

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SURVEY_DB_PATH` | No | `/var/db/survey/votes.duckdb` | Path to DuckDB file |
| `SURVEY_HTTP_ADDR` | No | `0.0.0.0:$PORT` | HTTP listen address |
| `SURVEY_ADMIN_TOKEN` | Yes | — | Token for admin API auth |
| `SURVEY_SITE_URL` | No | `https://pollmd.ssp.sh` | Public site URL for links |
