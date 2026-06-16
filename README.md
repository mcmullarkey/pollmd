# pollmd: A Minimal (Newsletter) Poll Tool That Works in Markdown

Pollmd is a ~200-line Go service that records anonymous reader ratings from newsletter
links into a [DuckDB](https://ssp.sh/brain/duckdb) file. Per-newsletter, per-answer, no cookies, no JS.
Query the results from your laptop over Quack.

Initial design doc: [`docs/prompts/initial/2026-06-04-newsletter-survey-design.md`](docs/prompts/initial/2026-06-04-newsletter-survey-design.md).

Docs site: **[pollmd.ssp.sh](https://pollmd.ssp.sh)**.

## Why pollmd?

- **Free and self-hosted.** Paid survey tools (Typeform, Polldaddy,
  SurveyMonkey) charge per-response or per-month after a small free tier.
  pollmd costs whatever your Railway / EC2 / Hetzner / FreeBSD box already
  costs — typically nothing extra. The DuckDB file grows by ~50 bytes per
  vote, so storage is a rounding error.
- **Markdown-native.** Polls are plain `[Label](URL)` links. They render in
  every newsletter platform that supports Markdown — no embeds, no
  JavaScript, no iframes, no platform lock-in. Your readers click a link
  the same way they click every other link in your newsletter.
- **Tiny.** ~200 lines of Go, one binary, one DuckDB file. Read the source
  in an afternoon. Fork it if you want different behaviour.
- **Privacy by construction.** No cookies, no JavaScript on the vote page,
  no fingerprinting. IP and User-Agent feed a hashed dedup key with a salt
  that rotates every midnight UTC; the salt is never persisted. After
  rotation, yesterday's hashes can't be reproduced — even with access to
  server logs.
- **Queryable with SQL.** The CSS bar-chart tally is nice, but the
  underlying DuckDB file is one `quack_query` away from any analysis you
  want to do — joins against your subscriber list, time-of-day patterns,
  whatever you can write in SQL.

## Features

- **Markdown link voting** — `[Label](https://<host>/<survey_id>/<answer>)`,
  one click records one vote, redirects to a "Thanks!" page.
- **Per-newsletter answer slugs** — invent any slugs you want per issue;
  the regex `^[a-z0-9][a-z0-9_-]{0,63}$` is the only constraint, no schema
  change, no allowlist required.
- **Optional answer locking** — `make survey-create SURVEY_ID=… ANSWERS=…`
  registers a per-survey allowlist. URL-fuzzers and curious readers see
  200s but nothing gets recorded; the rejection is logged with
  `answer-reject survey_id=… answer=…`.
- **Public landing page per survey** — `/{id}` renders a button per
  allowed answer for surveys registered via `survey-create`, so you can
  share one URL in a tweet/post instead of all four markdown links.
- **Server-rendered result page** — `/result/{id}` renders a CSS bar chart
  marked `noindex`. Whoever knows the slug can view; no DuckDB-WASM, no
  SQL endpoint exposed to the browser.
- **Privacy-respecting dedup** — `sha256(ip || ua || daily_salt ||
  survey_id)[:16]`. Salt is 32 random bytes, in-memory, rotated at
  midnight UTC, regenerated on every restart.
- **Bot filter** — substring match against ~40 User-Agent patterns
  (link unfurlers, search crawlers, RSS readers, security scanners,
  Safe Links). Empty UAs also skipped.
- **HEAD-prefetch tolerance** — Microsoft Safe Links and Gmail
  prefetchers get a 200 with no vote recorded.
- **One process, single writer** — Go HTTP server and Quack remote-read
  listener share one DuckDB connection. `SetMaxOpenConns(1)` is
  load-bearing.
- **Quack admin channel** — `make survey-result`, `make survey-reset`,
  ad-hoc SQL all flow over Quack on a separate port, token-authenticated.
  No SQL endpoint on the public HTTPS path.
- **Three deploy paths** — Railway (Docker + persistent volume),
  Linux/EC2/Hetzner (~10 lines + systemd), FreeBSD (the install script
  that runs on `ti`).

## What it looks like in a newsletter

```markdown
What did you think of today's newsletter?

[Awesome!](https://q.ssp.sh/2026-06-04/awesome)
[Pretty Good](https://q.ssp.sh/2026-06-04/good)
[Could be better](https://q.ssp.sh/2026-06-04/better)
[Worse](https://q.ssp.sh/2026-06-04/worse)

See the live tally → https://q.ssp.sh/result/2026-06-04
```

The path shape is `https://<host>/<survey_id>/<answer>`:

- `<survey_id>` identifies the newsletter issue (e.g. an ISO date like
  `2026-06-04`, or a slug like `weekly-42`).
- `<answer>` is whichever rating you want to record for that click
  (`awesome`, `good`, `better`, `worse`, `meh`, …).

Both slugs are free-form, validated against `^[a-z0-9][a-z0-9_-]{0,63}$`,
so the next newsletter can use entirely different `survey_id` and `answer`
slugs without any code or schema change. Each click records one vote and
redirects to a "Thanks!" page.

The legacy URL shape `https://<host>/survey/<survey_id>/<answer>` is kept
working so links shipped in past newsletters don't 404 — new newsletters
should use the shorter `/<survey_id>/<answer>` form.

### Locking answers per survey (optional, per-newsletter)

By default the server records any vote whose URL matches the slug regex —
useful when you want full flexibility to invent new answer slugs per
newsletter without touching code, env vars, or remembering to pre-register.

For surveys where you want to lock the answer space to a known set
(stops URL-fuzzers and curious readers from inventing slugs), register the
allowed answers **once before sending the newsletter**:

```sh
make survey-create SURVEY_ID=2026-06-15 ANSWERS=awesome,good,better,worse
```

That writes a row into the `surveys` table via the same Quack channel as
`survey-result` and `survey-reset` (same `SURVEY_QUACK_TOKEN`,
`RAILWAY_QUACK_HOST`, `RAILWAY_QUACK_PORT`). From that moment on, only the
listed answers count for that `survey_id`. Anything else returns 200 (so
the click still "succeeds" from the browser's perspective) but no row is
written. The skip is logged:

```
answer-reject survey_id=2026-06-15 answer=banana
```

So the four "official" markdown links keep working:

```markdown
[Awesome!](https://q.ssp.sh/2026-06-15/awesome)        ← counted
[Pretty Good](https://q.ssp.sh/2026-06-15/good)        ← counted
[Could be better](https://q.ssp.sh/2026-06-15/better)  ← counted
[Worse](https://q.ssp.sh/2026-06-15/worse)             ← counted
```

While these silently don't:

```
https://q.ssp.sh/2026-06-15/banana       ← rejected, logged
https://q.ssp.sh/2026-06-15/not-a-vote   ← rejected, logged
```

**The default is still wide open** — if you skip `survey-create`, that
survey behaves like it always has (any slug-valid answer counts). Mix and
match per newsletter:

```sh
# Important poll: lock it down
make survey-create SURVEY_ID=quarterly-review ANSWERS=keep,change,unsubscribe

# Quick experiment: skip survey-create, accept whatever
# (just send the markdown links and go)
```

Re-running `survey-create` upserts the row, so editing the allowed set is
just a re-run with new `ANSWERS=…`.

`make survey-create` prints a ready-to-paste block:

```
Landing page (share this URL):
  https://q.ssp.sh/2026-06-15

Markdown links to paste into your newsletter:

  [Awesome](https://q.ssp.sh/2026-06-15/awesome)
  [Good](https://q.ssp.sh/2026-06-15/good)
  [Better](https://q.ssp.sh/2026-06-15/better)
  [Not Sure](https://q.ssp.sh/2026-06-15/not-sure)
  [Worse](https://q.ssp.sh/2026-06-15/worse)

Live tally page:
  https://q.ssp.sh/result/2026-06-15
```

Override the host with `make survey-create … PUBLIC_URL=https://your.host`
(default is `https://q.ssp.sh`).

### Landing page (registered surveys only)

`https://<host>/<survey_id>` — and the alias `/survey/<survey_id>` —
renders a small page with a vote button per allowed answer. Useful for
sharing one URL in a tweet/post instead of all four. Slug → label
conversion: `not-sure` → `Not Sure`.

Only works for surveys registered via `make survey-create`. Unregistered
(open-mode) surveys 404, so there's no wildcard landing page for "guess
any slug".

### Per-survey results page

`https://<host>/result/<survey_id>` renders a small HTML page with a CSS
bar chart of the tally — same design as the `/thanks` page. The Go
handler reads from DuckDB and renders the bars server-side, so there's no
DuckDB-WASM and no query interface exposed to the browser. Whoever knows
the `survey_id` slug can view its results; nobody else can poke at the
DB. Marked `noindex` so it doesn't end up in search engines.


See [q.ssp.sh/result/init/](https://survey.ssp.sh/result/2026-06-04) as an example:
![image](static/images/result-example.webp)


## Architecture

Single Go process. `libduckdb` 1.5.3 is compiled in via
`duckdb-go-bindings/v2`, and the Quack extension is `INSTALL`'d / `LOAD`'d
at startup. **The same in-process DuckDB serves both sides** — HTTP click
writes and Quack remote reads share one writer, which is what DuckDB
needs (single-writer constraint).

```mermaid
flowchart LR
    subgraph readers["Newsletter readers"]
        B["Browser"]
    end

    subgraph laptop["You (laptop)"]
        M["make survey-create<br/>make survey-result<br/>make survey-reset"]
        D["local duckdb CLI<br/>+ quack extension"]
    end

    subgraph rly["Railway"]
        Edge["HTTPS edge · auto-TLS"]
        TCP["TCP Proxy · plaintext + token"]
        
        subgraph cont["pollmd container · single Go process (CGO)"]
            HTTP["net/http :8080<br/>vote / landing / result / thanks / style"]
            QSrv["Quack listener :9494<br/>started by CALL quack_serve"]
            DB[("libduckdb 1.5.3<br/>single writer · in-process")]
            Salt["32-byte salt in memory<br/>rotates @ UTC midnight"]
        end
        
        V[("Persistent volume<br/>votes.duckdb")]
    end

    B -->|"GET /{id}/{answer}"| Edge
    Edge --> HTTP
    HTTP -->|"RecordVote, TallyBySurvey, GetAllowedAnswers"| DB
    HTTP -.->|"voter.Hash"| Salt
    M -->|"survey-create writes via Quack"| TCP
    D -->|"quack_query over HTTP"| TCP
    TCP --> QSrv
    QSrv -->|"reads/writes in same process"| DB
    DB --> V
```

**Write path** (newsletter click): browser → Railway HTTPS edge → Go
`net/http` mux → `handleSurvey` → `store.RecordVote` → in-process
`libduckdb` → `votes.duckdb` on the persistent volume. Go and DuckDB
are not separate services; CGO links the two.

**Read path** (your laptop queries): local `duckdb` CLI →
`quack_query(…)` over HTTP → Railway TCP Proxy (plaintext, token
authenticates) → Quack listener on container port 9494 → same
`libduckdb` in the same process → same file. Quack is never used for
recording votes; it's the read-side / admin channel.

The only thing `make survey-create` / `survey-reset` / `survey-result`
use Quack for **is admin SQL** (INSERT into `surveys`, DELETE from
`votes`, SELECT for the bar-chart tally). Vote recording always goes
through HTTP.

### One vote, end to end

```mermaid
flowchart TD
    Click["Browser clicks<br/>https://q.ssp.sh/2026-06-04/awesome"]
    Click --> Edge["Railway HTTPS edge"]
    Edge --> H["Go handleSurvey()"]
    H --> Method{"HTTP method?"}
    
    Method -->|"HEAD (Safe Links prefetch)"| OK1["200 · no record"]
    Method -->|"GET"| Slug{"slug regex<br/>matches id + answer?"}
    
    Slug -->|"no"| Err["400 Bad Request"]
    Slug -->|"yes"| Bot{"User-Agent<br/>looks like a bot?"}
    
    Bot -->|"TwitterBot, etc."| OK2["200 · log bot-skip"]
    Bot -->|"browser"| Reg{"survey_id<br/>in surveys table?"}
    
    Reg -->|"no (open mode)"| Hash["voter = sha256(ip + ua + daily_salt + survey_id)[:16]"]
    Reg -->|"yes and answer allowed"| Hash
    Reg -->|"yes and answer NOT allowed"| OK3["200 · log answer-reject"]
    
    Hash --> Up[("INSERT INTO votes<br/>ON CONFLICT (survey_id, voter)<br/>DO UPDATE — last vote wins")]
    Up --> Redir["302 → /thanks · log vote"]
```

Key consequences of this shape:

- **No second writer.** Quack runs *inside* the same Go process as the
  HTTP server, so there's exactly one thing writing to
  `votes.duckdb` — required by DuckDB.
- **One click = at most one row per (survey_id, voter) per day.** Re-clicks
  upsert. The daily salt rotation is the dedup window — after midnight
  UTC, the salt regenerates and the same reader produces a different
  hash for the same survey, so they could vote again. That's a feature
  for multi-day polls and the price of not persisting any identifier.
- **The landing page (`/{id}`) and the markdown links resolve to the
  same handler.** Clicking a button on the landing page hits the same
  `/{id}/{answer}` route as the newsletter link → same voter hash →
  same row → same upsert.

## How votes are deduplicated

`voter = sha256(ip || ua || daily_salt || survey_id)[:16]` (hex).

- The daily salt is 32 random bytes generated in memory at startup, rotated
  every midnight UTC, and regenerated on every process restart. It is
  **never written to disk**.
- After rotation, yesterday's hashes can no longer be reproduced from logs.
- Including `survey_id` in the hash means the same reader produces different
  hashes for different newsletters, so cross-issue tracking is impossible.

If the same reader clicks twice on the same newsletter (e.g. Awesome, then
Good), the second click replaces the first — last vote wins.

## One-time server setup

The server needs Go, a `libduckdb` available to the linker (or a built-in
one via the Go bindings on Linux), an env file with a generated Quack
token, and a service supervisor. Pick your platform:

- **[Railway](docs/content/docs/install/railway.md)** — Docker-based, one service,
  persistent volume for the DuckDB file. HTTP on Railway's HTTPS edge,
  Quack exposed via TCP Proxy so you can `ATTACH` from your laptop without
  custom DNS up front.
- **[Linux (EC2 / Hetzner / anywhere)](docs/content/docs/install/linux.md)** — much
  shorter. `duckdb-go-bindings/v2` ships a prebuilt `libduckdb` for Linux,
  so `go build` Just Works. ~10 lines of shell + a systemd unit.
- **[FreeBSD](docs/content/docs/install/freebsd.md)** — what I actually run on `ti`.
  Needs a from-source DuckDB build (~20 min) because upstream ships no
  FreeBSD binaries. Automated via `make push-installer` → `make install-on-server`.

> [!NOTE]
> FreeBSD because I already have one running on my self-hosted server, so it
> costs me nothing extra. If I were starting fresh, EC2 with the Linux guide
> would be ~$3-7/mo and would skip the source build entirely.

## Reverse proxy + TLS (external)

TLS termination happens on whatever reverse proxy is in front of `ti`
(e.g. Nginx Proxy Manager on Unraid). Add two proxy hosts with Let's Encrypt:

| Hostname                       | Backend                         |
|--------------------------------|---------------------------------|
| `survey.sspaeti.duckdns.org`   | `http://<ti-LAN-ip>:8080`       |
| `quack.sspaeti.duckdns.org`    | `http://<ti-LAN-ip>:9494`       |

The `survey.*` host carries the click traffic; the `quack.*` host carries
the DuckDB Quack remote-protocol traffic for ad-hoc queries from your laptop.
Restrict the two ports to LAN-only on `ti`'s firewall — they shouldn't be
reachable from the public internet directly.

## Deploy

From your laptop, in this directory:

```sh
make deploy
```

This rsyncs the source to the host, builds the Go binary there, atomically
swaps `/usr/local/bin/survey`, and restarts the service. On FreeBSD the build
links dynamically against the system `libduckdb.so` via `-tags=duckdb_use_lib`;
on Linux the prebuilt library inside `duckdb-go-bindings/v2` is used and no
extra tag is needed.

Run `make help` for the full target list. Common ones:
`make smoke` (DNS + TLS + healthz), `make logs`, `make status`,
`make token`, `make duckdb-connect`.

## Query from your laptop

### Fastest: rendered tally with bar chart

```sh
export SURVEY_QUACK_TOKEN='<token from Railway env>'
export RAILWAY_QUACK_HOST='XXXXX.proxy.rlwy.net'    # your TCP Proxy host
export RAILWAY_QUACK_PORT='99999'                    # your TCP Proxy port
```


Show all survey result
```sh
make survey-result                          # all surveys
```

Example output — bars scale to each survey's top answer, so within-newsletter
proportions are visible at a glance:

```
┌────────────┬─────────┬────────┬────────────────────────────────┐
│ survey_id  │ answer  │ clicks │             chart              │
│  varchar   │ varchar │ int64  │            varchar             │
├────────────┼─────────┼────────┼────────────────────────────────┤
│ 2026-06-11 │ awesome │     42 │ ██████████████████████████████ │
│ 2026-06-11 │ good    │     27 │ ███████████████████▎           │
│ 2026-06-11 │ better  │      8 │ █████▋                         │
│ 2026-06-04 │ awesome │     38 │ ██████████████████████████████ │
│ 2026-06-04 │ good    │     22 │ █████████████████▎             │
│ 2026-06-04 │ better  │     11 │ ████████▋                      │
│ 2026-06-04 │ worse   │      2 │ █▌                             │
└────────────┴─────────┴────────┴────────────────────────────────┘
```
Or one specific:
```sh
make survey-result SURVEY_ID=2026-06-04     # one newsletter only
```

Looks like this:
```
┌────────────┬────────────┬────────┬────────────────────────────────┐
│ survey_id  │   answer   │ clicks │             chart              │
│  varchar   │  varchar   │ int64  │            varchar             │
├────────────┼────────────┼────────┼────────────────────────────────┤
│ 2026-06-04 │ worse      │      2 │ ██████████████████████████████ │
│ 2026-06-04 │ best       │      1 │ ███████████████                │
└────────────┴────────────┴────────┴────────────────────────────────┘
```


### Interactive: ad-hoc SQL on the remote DuckDB

```sh
make railway-duckdb-connect       # for Railway TCP Proxy host:port
# — or —
make duckdb-connect               # for the FreeBSD path with custom DNS
```

`railway-duckdb-connect` drops you at a duckdb prompt with two helpers
pre-loaded:

- `remote_votes` — view over the remote `votes` table
- `rq(sql)` — table macro that runs arbitrary SQL on the remote

```sql
-- Latest 20 clicks
FROM remote_votes ORDER BY ts DESC LIMIT 20;

-- Filter locally after fetching the table
FROM remote_votes WHERE survey_id = '2026-06-04';

-- Aggregate on the server, return small result
FROM rq('SELECT survey_id, answer, count(*) AS n
         FROM votes GROUP BY ALL
         ORDER BY survey_id DESC, n DESC');
```

> [!NOTE]
> The Makefile wraps everything in `quack_query` because `ATTACH 'quack:…'`
> errors with `Binder Error: Catalog "s" does not exist!` in the Quack build
> shipped with DuckDB 1.5.3 (extension build `1693647`). When the next quack
> release lands the helpers will switch to a proper `ATTACH`.

### Fallback paths

- **Inside Railway's container:** open a shell from the dashboard, then
  `curl https://install.duckdb.org | sh` and
  `~/.duckdb/cli/latest/duckdb -readonly /var/db/survey/votes.duckdb`.
- **FreeBSD:** `ssh ti "duckdb /var/db/survey/votes.duckdb -c 'FROM votes'"`.

## Privacy

- No cookies, no JavaScript, no fingerprinting.
- IP and User-Agent are read on each request, fed into the voter hash, and
  immediately discarded. Nothing identifying is persisted.
- The daily salt rotation means past hashes cannot be reproduced — even with
  access to server logs.
- Access logs record only `survey_id` and `answer`.

## Layout

```
.
├── cmd/survey/main.go             # entrypoint, env wiring
├── internal/
│   ├── server/server.go           # routes, vote + result + thanks handlers, bot UA filter
│   ├── server/thanks.html         # embedded thanks page (uses /style.css)
│   ├── server/result.html         # embedded result page (uses /style.css)
│   ├── server/style.css           # shared stylesheet, served at /style.css
│   ├── store/store.go             # DuckDB open, schemas, quack_serve, tallies, allowlist
│   └── voter/hash.go              # daily salt + voter hash
├── deploy/
│   ├── railway/Dockerfile         # Railway image (multi-stage Go build → debian-slim)
│   ├── install-on-server.sh       # idempotent FreeBSD installer (runs as root on ti)
│   ├── survey.rc                  # FreeBSD rc.d service script
│   └── survey.env.example         # env-var template
├── docs/                          # Hugo + Hextra site → pollmd.ssp.sh
│   ├── hugo.yaml
│   ├── content/
│   │   ├── _index.md              # docs home (feature grid)
│   │   └── docs/
│   │       ├── _index.md          # Overview & philosophy
│   │       ├── usage.md           # markdown link shape, answer locking
│   │       ├── architecture.md
│   │       ├── querying.md
│   │       ├── privacy.md
│   │       ├── faq.md
│   │       └── install/
│   │           ├── railway.md     # Railway one-time setup
│   │           ├── linux.md       # minimal Linux/EC2 guide (the easy path)
│   │           └── freebsd.md     # full FreeBSD guide
│   ├── layouts/shortcodes/readme-section.html   # pulls heading-bounded README sections
│   └── prompts/initial/           # initial AI design spec(s)
├── railway.json                   # Railway config-as-code (auto-detected at repo root)
├── .dockerignore                  # trims the Railway build context
├── Makefile
├── CHANGELOG.md
└── go.mod
```

## Changelog

See [`CHANGELOG.md`](CHANGELOG.md) for the running history of features
and behaviour changes — Railway path, the short `/<id>/<answer>` URL
shape, the `/result/{id}` page, the bot User-Agent filter, the
per-survey answer-locking via `make survey-create`, and so on.
