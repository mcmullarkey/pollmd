# Landing page + Hugo docs site

**Date:** 2026-06-16
**Status:** Design — not yet implemented

## Why

`https://q.ssp.sh/` currently returns 404. Newsletter readers who chop the
URL down to the host (curious clicks, share-the-link reflex) hit a dead page
with no explanation of what they just voted on. pollmd is also a public
project at `github.com/sspaeti/pollmd` and would benefit from a proper docs
site at `pollmd.ssp.sh` — same shape as `neomd.ssp.sh`.

This spec covers three pieces that ship together:

1. A minimal **landing page** at `q.ssp.sh/` served by the Go binary.
2. A **Hugo + Hextra docs site** at `pollmd.ssp.sh`, sharing text with the
   project README via a `readme-section` shortcode.
3. **README additions** (Features, Why pollmd?) so the docs site has a
   single source of truth for marketing-style content.

The README stays the canonical text — edits there flow into the docs site
automatically. Nothing in this spec adds runtime dependencies, changes the
DuckDB schema, or touches the vote-recording path.

## Out of scope

- Search functionality beyond Hextra's default in-site search.
- A CLI install one-liner / Homebrew tap / AUR package.
- Translating any existing content to other languages.
- Changing how votes are recorded, deduplicated, or stored.

## Piece 1: Landing page at `/`

### Route

Add a fourth public HTTP route in `internal/server/server.go`:

```go
const routeHome = "/{$}" // matches exactly "/" — Go 1.22 anchor
```

The `{$}` end-of-pattern anchor (Go 1.22) ensures `/` matches *only* the
root, never `/{id}` or `/{id}/{answer}`. Without it the more-specific
`/{id}` would still win for non-root paths, but adding `{$}` makes intent
explicit and avoids surprises if anyone ever introduces a different root
handler.

Registration order in `ListenAndServe`:

```go
mux.HandleFunc(routeHome, s.handleHome)
```

### Handler

```go
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodHead { w.WriteHeader(http.StatusOK); return }
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
    }
    base := publicBaseURL(r)
    data := homePageData{
        PageURL:       base + "/",
        OGImageURL:    base + routeOGImage,
        OGTitle:       "pollmd — minimal newsletter polls in Markdown",
        OGDescription: "A ~200-line Go service that records anonymous newsletter reader ratings into a DuckDB file. No cookies, no JS.",
        DocsURL:       "https://pollmd.ssp.sh/",
        GitHubURL:     "https://github.com/sspaeti/pollmd",
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Header().Set("Cache-Control", "no-store")
    if err := s.home.Execute(w, data); err != nil {
        log.Printf("home template: %v", err)
    }
}
```

`DocsURL` and `GitHubURL` are hardcoded constants for now (one place to
edit if they ever change). They could move to `server.Config` later if a
fork/reuse case appears — not worth the env wiring on day one.

### Template

New file `internal/server/home.html`, added to the existing
`//go:embed thanks.html result.html landing.html style.css ogimage.png`
directive. Loaded in `New()` alongside the other templates:

```go
home: template.Must(template.ParseFS(staticFS, "home.html")),
```

Content (drafted from README phrasing — short, concrete, same voice as
`landing.html`):

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>pollmd — minimal newsletter polls in Markdown</title>
<meta name="description" content="{{.OGDescription}}">
<!-- og: + twitter: tags, same shape as landing.html -->
<link rel="stylesheet" href="/style.css">
</head>
<body>
  <main class="card">
    <div class="hero">
      <img class="logo" src="https://www.ssp.sh/brain/logo_ssp_main.png" alt="sspaeti.com logo">
      <h1>pollmd</h1>
      <p class="subtitle">Minimal newsletter polls in Markdown.</p>
    </div>

    <p>
      A ~200-line Go service that records anonymous reader ratings into a
      single <a href="https://ssp.sh/brain/duckdb">DuckDB</a> file.
      Per-newsletter, per-answer, no cookies, no JS. Query the results
      from your laptop over Quack.
    </p>

    <div class="answers">
      <a class="answer-link" href="{{.DocsURL}}">Docs &rarr;</a>
      <a class="answer-link" href="{{.GitHubURL}}">GitHub &rarr;</a>
    </div>

    <p class="muted">
      No cookies, no tracking. Each request is hashed with a salt that
      rotates every midnight UTC.
    </p>
  </main>
</body>
</html>
```

Reuses the existing CSS classes (`card`, `hero`, `subtitle`, `answers`,
`answer-link`, `muted`) — no CSS changes needed. The `Created with pollmd`
footer-credit is dropped here because the user is *on* pollmd already.

### Tests

Add to `internal/server/server_test.go` (create if missing):

- `GET /` → 200, body contains "pollmd" and a link to
  `https://github.com/sspaeti/pollmd`.
- `HEAD /` → 200, no body.
- `GET /` does NOT shadow `GET /someslug` — sending `GET /init` should
  still reach the landing handler (or 404 for unregistered).
- `GET /` does NOT shadow `GET /init/awesome` — vote handler must still
  win for two-segment paths.

The shadowing tests pin the `{$}` anchor: if anyone removes it during
refactor, these tests catch it.

## Piece 2: Hugo + Hextra docs at `docs/`

### Final tree

```
docs/
├── hugo.yaml
├── go.mod
├── Makefile
├── README.md                          # 5-line "how to develop the docs"
├── content/
│   ├── _index.md                      # home: hero + feature-grid + cards
│   └── docs/
│       ├── _index.md                  # overview & philosophy
│       ├── usage.md                   # markdown link shape, answer locking
│       ├── architecture.md            # pulls README "## Architecture"
│       ├── querying.md                # pulls README "## Query from your laptop"
│       ├── privacy.md                 # pulls README "## Privacy"
│       ├── install/
│       │   ├── _index.md              # picker: Railway vs Linux vs FreeBSD
│       │   ├── railway.md             # ← moved from docs/install-railway.md
│       │   ├── linux.md               # ← moved from docs/install-linux.md
│       │   └── freebsd.md             # ← moved from docs/install-freebsd.md
│       └── faq.md
├── layouts/shortcodes/
│   └── readme-section.html
├── static/
│   └── images/
│       └── result-example.webp        # copied from ../static/images/
└── prompts/
    ├── initial/
    │   └── 2026-06-04-newsletter-survey-design.md   # ← moved from docs/superpowers/specs/
    └── 2026-06-16-landing-page-and-docs-site-design.md  # this spec
```

`docs/install-railway.md`, `install-linux.md`, `install-freebsd.md` are
**moved** (not copied) into `docs/content/docs/install/`. README and
CLAUDE.md links that reference the old paths get updated in the same
change.

The original `docs/superpowers/specs/2026-06-04-newsletter-survey-design.md`
moves to `docs/prompts/initial/2026-06-04-newsletter-survey-design.md` —
matches the spirit of neomd's `docs/initial-prompt/`. The
`docs/superpowers/` directory then gets deleted.

The Hugo project treats `docs/prompts/` and `docs/superpowers/specs/` as
not-rendered — Hugo only walks `content/`, `layouts/`, `static/`,
`assets/`, etc. So putting prompt/design files under `docs/` doesn't end
up serving them from `pollmd.ssp.sh`.

### `hugo.yaml`

Copy neomd's verbatim, change three fields:

```yaml
title: pollmd
baseURL: "https://pollmd.ssp.sh/"
canonifyURLs: false

disableKinds: ["RSS"]

module:
  imports:
    - path: github.com/imfing/hextra

markup:
  goldmark:
    renderer: { unsafe: true }
    parser:
      attribute: { block: true }
      autoHeadingID: true
      autoHeadingIDType: github
  highlight:
    noClasses: false
  tableOfContents:
    startLevel: 2
    endLevel: 6
    ordered: false

menu:
  main:
    - { name: Documentation, pageRef: /docs,  weight: 1 }
    - { name: Search, weight: 2, params: { type: search } }
    - { name: GitHub, weight: 3, url: "https://github.com/sspaeti/pollmd",
        params: { icon: github } }
```

### `go.mod`

```
module github.com/sspaeti/pollmd/docs

go 1.24

require github.com/imfing/hextra v0.12.2 // indirect
```

### `Makefile`

Copy neomd's verbatim — `serve` (port 1311), `build`, `clean`, `help`.

### `layouts/shortcodes/readme-section.html`

Copy neomd's verbatim, with one change: the default `file` parameter
changes from `"../../README.md"` to `"../README.md"` because the Hugo
project root in this repo is at `docs/`, one level below the project
README:

```go-html-template
{{- $file := .Get "file" | default "../README.md" -}}
{{- $start := .Get "start" -}}
{{- $end := .Get "end" -}}
{{- $content := readFile $file -}}

{{- if and $start $end -}}
  {{- $lines := split $content "\n" -}}
  {{- $inSection := false -}}
  {{- $output := slice -}}
  {{- range $lines -}}
    {{- if strings.Contains . $start -}}{{- $inSection = true -}}{{- end -}}
    {{- if $inSection -}}{{- $output = $output | append . -}}{{- end -}}
    {{- if strings.Contains . $end -}}{{- $inSection = false -}}{{- end -}}
  {{- end -}}
  {{- $output | delimit "\n" | markdownify -}}
{{- else -}}
  {{- $content | markdownify -}}
{{- end -}}
```

**Path note:** Hugo `readFile` resolves relative to the Hugo project root
(`docs/`), so `../README.md` is the project's top-level README.

### `content/_index.md` (home page)

Hextra hero + feature grid + doc-card grid. Content drafted from README
in your voice. Feature cards (six, matching neomd's pattern):

| Title                       | Subtitle (drafted from README phrasing)                                                                                                       |
|-----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| Markdown-native links       | Just paste `[Awesome](https://q.ssp.sh/2026-06-04/awesome)` into your newsletter. One click, one vote.                                        |
| No cookies, no JS           | No fingerprinting. IP + UA + salt are hashed and immediately discarded. Salt rotates every midnight UTC.                                      |
| One binary, one file        | A ~200-line Go service writing into one DuckDB file. No external DB, no Redis, no JS bundle.                                                  |
| Per-newsletter flexibility  | Invent any answer slug per issue — `awesome`, `meh`, `keep`, `unsubscribe`. Or lock the allowed set per survey with `make survey-create`.     |
| SQL from your laptop        | DuckDB + Quack gives you `make survey-result` for a bar-chart tally, or drop into a `duckdb` prompt for ad-hoc queries.                       |
| Self-host anywhere          | Railway, Linux/EC2/Hetzner, or FreeBSD — each platform has a guide. Vote endpoint + Quack admin channel share one process.                    |

Hero subtitle: *"A ~200-line Go service that records anonymous reader
ratings into a DuckDB file — per-newsletter, per-answer, no cookies, no
JS."*

Hero CTA button: *"Get started"* → `/docs/install/`.

Below the feature grid: a `{{< cards cols="3" >}}` grid linking to
Overview, Install, Usage, Architecture, Querying, FAQ — mirroring
neomd's home.

A small "Initial AI spec" link at the bottom points at
`/prompts/initial/2026-06-04-newsletter-survey-design.md` on GitHub
(raw URL or repo blob URL — defer to implementation which one renders
better). Lets curious readers see where the project started from.

### `content/docs/_index.md` (overview)

Title: *"Overview & Philosophy"*.

Short intro paragraph in your voice (drafted from README's first 7
lines), then the `{{< readme-section start="## Why pollmd?" end="## Features" >}}`
pull. README is the source of truth here.

### `content/docs/usage.md`

How to write the markdown links for a newsletter. Pulls the README
section "## What it looks like in a newsletter" through the next
heading. Adds a short intro paragraph specific to the docs site.

### `content/docs/architecture.md`

Pulls README `## Architecture` through `## One-time server setup` (the
Mermaid diagrams come along for the ride — Hextra supports Mermaid via
goldmark). No new content; just a thin wrapper.

### `content/docs/querying.md`

Pulls README `## Query from your laptop` through `## Privacy`.

### `content/docs/privacy.md`

Pulls README `## Privacy` through `## Layout`.

### `content/docs/install/_index.md`

Short picker page — three cards linking to railway.md, linux.md,
freebsd.md, each with a one-liner from the existing install guides'
opening paragraphs. The full content lives in the individual files (the
existing install guides, moved as-is).

### `content/docs/install/railway.md`, `linux.md`, `freebsd.md`

Moved verbatim from `docs/install-railway.md`, `install-linux.md`,
`install-freebsd.md`. Add Hugo frontmatter (`title`, `weight`). Audit
internal links — they currently reference each other as
`install-railway.md` style relative paths; update to `./railway` or
`/docs/install/railway` style.

### `content/docs/faq.md`

Short, three to five Q/A pairs drafted from things that already come up
in README + CLAUDE.md:

- *"Why DuckDB and not Postgres / SQLite / a hosted DB?"*
- *"Is `survey_id` opaque or do you parse the date?"*
- *"How do I delete an accidental vote?"* → `make survey-reset`.
- *"Can I run it without Quack exposed publicly?"*
- *"What's the cost? Free vs. Typeform / Polldaddy / etc.?"*

## Piece 3: README additions

Two new sections inserted between the opening paragraphs and the existing
`## What it looks like in a newsletter`. Both wrapped with start/end
markers so the Hugo shortcode can pull them cleanly:

### `## Why pollmd?`

Five-bullet why-this-exists list:

- **Free and self-hosted.** Paid survey tools (Typeform, Polldaddy, etc.)
  cost per-response after the free tier. pollmd costs whatever your
  Railway/EC2/FreeBSD box already costs — typically nothing extra.
- **Markdown-native.** Polls are plain `[Label](URL)` links. They render
  in every newsletter platform that supports Markdown — no embeds, no
  JavaScript, no iframes, no platform lock-in.
- **Tiny.** ~200 LOC of Go, one binary, one DuckDB file. Read the source
  in an afternoon. Fork it if you want different behaviour.
- **Privacy by construction.** No cookies, no JavaScript on the vote
  page, no fingerprinting. IP + UA feed a hashed dedup key with a salt
  that rotates every midnight UTC; the salt is never persisted.
- **Queryable with SQL.** Tally bars are nice, but the underlying
  DuckDB file is one `quack_query` away from any analysis you want to
  do — joins against your subscriber list, time-of-day patterns,
  whatever.

### `## Features`

Twelve-ish bullet list extracted from what's already scattered through
the README:

- **Markdown link voting** — `[Label](https://<host>/<survey_id>/<answer>)`,
  one click records one vote.
- **Per-newsletter answer slugs** — invent any slugs you want per issue;
  the regex (`^[a-z0-9][a-z0-9_-]{0,63}$`) is the only constraint, no
  schema change, no allowlist required.
- **Optional answer locking** — `make survey-create SURVEY_ID=…
  ANSWERS=…` per-survey allowlist; URL-fuzzers and curious readers see
  200s but nothing gets recorded.
- **Public landing page per survey** — `/{id}` shows a button per
  allowed answer for surveys created via `survey-create`, so you can
  share one URL instead of four markdown links.
- **Server-rendered result page** — `/result/{id}` renders a CSS bar
  chart with `noindex`. Whoever knows the slug can view; no DuckDB-WASM,
  no SQL endpoint exposed.
- **Privacy-respecting dedup** — `sha256(ip || ua || daily_salt ||
  survey_id)[:16]`. Salt is 32 random bytes, in-memory, rotated at
  midnight UTC, regenerated on every restart.
- **Bot filter** — substring match against ~40 User-Agent patterns
  (link unfurlers, search crawlers, RSS readers, security scanners,
  Safe Links). Empty UA also skipped.
- **HEAD prefetch tolerance** — Microsoft Safe Links and Gmail
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

(Final phrasing pulled from README's existing prose so voice is
consistent. The list above is the spec-side outline; actual README
writing happens in implementation.)

## Piece 4: GitHub Actions deploy

New file `.github/workflows/deploy-docs.yaml`, copied from neomd's
verbatim except for:

```yaml
on:
  push:
    branches: ["main"]      # neomd has "main" + "neomd-docs"; we only need main
    paths:
      - 'docs/**'
      - '.github/workflows/deploy-docs.yaml'
```

Manual one-time steps (documented in the spec, not automated):

1. **Enable GitHub Pages** in the `sspaeti/pollmd` repo settings → Pages
   → Source: GitHub Actions.
2. **Add CNAME**: in the repo root (not `docs/static/`), create a file
   called `CNAME` containing `pollmd.ssp.sh`. Or set it via the Pages UI;
   either way commits the same file.
3. **DNS**: add a CNAME record for `pollmd.ssp.sh` → `sspaeti.github.io`
   in the DNS provider for `ssp.sh`.
4. **TLS**: GitHub Pages provisions Let's Encrypt automatically once DNS
   propagates.

Until DNS is set, the workflow still runs and the docs are reachable at
`sspaeti.github.io/pollmd/` — the spec mentions this as a fallback.

## Piece 5: ancillary edits

- **`README.md`**: paths to install guides change from
  `docs/install-*.md` to `docs/content/docs/install/*.md` (in the
  `One-time server setup` and `Layout` sections). The repo tree diagram
  inside README needs an update too. The line *"Initial design doc lives
  at `docs/superpowers/specs/2026-06-04-newsletter-survey-design.md`"*
  becomes *"Initial design doc lives at
  `docs/prompts/initial/2026-06-04-newsletter-survey-design.md`"*.
- **`CLAUDE.md`**: references to `docs/install-railway.md`,
  `install-linux.md`, `install-freebsd.md`, and the *"Initial design doc
  lives at `docs/superpowers/specs/...`"* line. The `make sync` exclude
  list stays `docs/`, so deploy behaviour doesn't change.
- **`Makefile`**: nothing changes for the Go build/deploy targets. The
  Hugo `make serve` / `make build` lives in `docs/Makefile` only — the
  root Makefile doesn't need to know about it.
- **`docs/superpowers/`** directory: deleted after the two files inside
  it move to `docs/prompts/`.

## Testing

### Landing page

- `go test ./internal/server -run TestHandleHome` covers the four cases
  above (GET, HEAD, no-shadow on landing, no-shadow on vote).
- Smoke test on the live Railway service: `curl -s
  https://q.ssp.sh/` should return the new HTML; `curl -sI
  https://q.ssp.sh/2026-06-04/awesome` should still 302 to `/thanks`.

### Hugo build

- `cd docs && hugo --gc --minify` runs clean (no template errors, no
  broken-link warnings).
- `cd docs && make serve` boots Hugo at `localhost:1311` and renders
  every page. Visual check: home, install pickers, each install guide,
  usage, architecture (Mermaid renders), querying, privacy, FAQ.
- Shortcode pulls: edit a section in `README.md`, refresh docs page,
  confirm the change appears.

### CI

- The deploy workflow triggers on the first push to `main` that touches
  `docs/**`. Check the Actions tab for green. The deployed URL is
  `sspaeti.github.io/pollmd/` until DNS is up; then `pollmd.ssp.sh/`.

## Risks and rollback

- **`{$}` route anchor mistake** would either shadow vote URLs or leave
  `/` returning 404 still. The four route-shadowing tests cover both
  failure modes.
- **Hextra version drift**: `hextra v0.12.2` is what neomd pins. If a
  newer version breaks the build, pin to a known-good tag rather than
  tracking latest.
- **README ↔ shortcode coupling**: if someone removes a section marker
  (e.g. renames `## Features` to `## What's included`), the shortcode
  pull goes empty. Mitigation: keep the markers stable and treat them
  like API. The spec lists which sections are pulled, so a grep finds
  the dependencies.
- **GitHub Pages DNS propagation**: setting `pollmd.ssp.sh` CNAME takes
  minutes to hours. Until then, the landing page's `Docs →` button hits
  a not-yet-resolved hostname. Acceptable — pollmd.ssp.sh is the target;
  if you want a placeholder, the spec is the place to add it.

Rollback for each piece is independent:

- Landing page: revert the `home.html` + handler commit, redeploy.
- Hugo docs: delete `docs/content/`, `docs/hugo.yaml`, `docs/go.mod`,
  `docs/Makefile`, `docs/layouts/`, `docs/static/`. Move install guides
  back to `docs/install-*.md`. Move the spec files back to
  `docs/superpowers/specs/`. Revert the workflow.
- README additions: revert the README commit. Existing readers see the
  README they remember.

## Open questions (none blocking)

- *Should the landing page link to a Twitter/X profile?* — Out of
  scope for this pass; the existing twitter handle is in og:twitter:site
  on the result page already, that's enough for now.
- *Should we add a Google Analytics / Plausible tag on pollmd.ssp.sh?*
  — Out of scope; the project's whole pitch is "no tracking", a tracker
  on the docs site would be a weird signal. Defer.
