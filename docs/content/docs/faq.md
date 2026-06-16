---
title: FAQ
weight: 70
---

### Why DuckDB and not Postgres / SQLite / a hosted DB?

DuckDB is a single-file embedded database with first-class SQL and zero
operational overhead — no separate server, no auth surface, no schema
migrations to coordinate. Combined with the [Quack](https://duckdb.org/docs/stable/core_extensions/quack/overview.html)
extension, the *same* file the Go process writes can be queried remotely
from your laptop's `duckdb` CLI. SQLite would work but lacks the remote-read
ergonomics; Postgres or a hosted DB would add a network hop, credentials,
and a separate operational thing to keep alive for what is fundamentally a
tiny single-writer workload.

### Is `survey_id` opaque, or do you parse the date?

Opaque. The server validates it against `^[a-z0-9][a-z0-9_-]{0,63}$` and
treats it as a string. ISO-style dates (`2026-06-04`) work, slugs
(`weekly-42`) work, anything else that matches the regex works. The date
format is a convention, not enforced.

### How do I delete an accidental vote?

Per-survey wipe via the Quack admin channel:

```sh
make survey-reset SURVEY_ID=<id> CONFIRM=yes
```

The `CONFIRM=yes` gate is there so a stray re-run of `make survey-reset` in
shell history doesn't quietly wipe a different survey. There's no per-row
delete by design — votes are deduplicated by hash, but the hash itself is
designed to be unreversible after the daily salt rotates.

### Can I run it without Quack exposed publicly?

Yes. The Quack listener is on a separate port (`SURVEY_QUACK_ADDR`, default
`:9494`). On the FreeBSD path, this is bound LAN-only and reached via a
custom DNS host. On Railway, the TCP Proxy adds a public hostname, but
authentication is by token (a base64 32-byte secret) — no token, no
connection. You can also skip the Quack proxy entirely on Railway and shell
into the container to query the file directly. See
[Querying](../querying/) for the alternatives.

### What's the cost? Free vs. Typeform / Polldaddy / etc.?

Paid survey tools (Typeform, Polldaddy, SurveyMonkey, etc.) usually have a
free tier capped at a few responses, then charge per-response or per-month.
pollmd costs whatever your existing Railway/EC2/Hetzner/FreeBSD box costs —
typically nothing extra. The DuckDB file grows by ~50 bytes per vote, so
storage is a rounding error.
