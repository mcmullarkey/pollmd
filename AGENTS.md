# AGENTS.md — Project Context for the Agent Framework

> Customize this for your project: fill in the placeholders and add project-specific details.

## Project Overview

**Project:** pollmd
**Language:** Go 1.24 (CGO for DuckDB)
**Module:** `github.com/sspaeti/minimal-newsletter-survey`
**Description:** A minimal newsletter poll tool that records anonymous reader ratings from
markdown links into DuckDB. ~200 lines of Go, one binary, one DuckDB file.
No cookies, no JS, no fingerprinting. Three deploy paths: Railway (Docker),
Linux, FreeBSD.

**Key dependencies:**
- `github.com/duckdb/duckdb-go/v2` — DuckDB Go bindings (CGO)
- Quack extension for remote DuckDB access (admin channel)
- No framework — standard `net/http` mux

## Agent System

This project uses a multi-agent system for spec-first development. See `~/.config/opencode/agents/` for agent definitions.

### Agents

| Agent | Role | Model | Mode |
|-------|------|-------|------|
| **@director** | Orchestrator — manages spec-to-PR pipeline | DeepSeek V4 Flash Free | primary |
| **@speculator-a** | Minimal-sufficient spec proposer | DeepSeek V4 Flash Free | subagent |
| **@speculator-b** | Adversarial-sufficient spec proposer | Nemotron 3 Ultra Free | subagent |
| **@resolver** | Per-AC merge/pick between proposals | Big Pickle | subagent |
| **@builder** | BDD TDD implementor | DeepSeek V4 Flash Free | subagent |
| **@gatekeeper** | Commit-level adversarial reviewer | MiMo-V2.5 Free | subagent |
| **@pr-reviewer** | Per-PR adversarial reviewer | North Mini Code Free | subagent |

### Pipeline

```
Feature Request
  → Director decomposes into ACs
  → Per AC: @speculator-a + @speculator-b (parallel) → @resolver (merge/pick)
  → Batch-clarify disagreements (if any)
  → gh issue create (one per AC) — CHECKPOINT
  → Build dependency DAG → batch implementation
  → Per issue: @builder → @gatekeeper (per commit) → PR → @pr-reviewer → merge
```

### Key Principles

- **Director delegates everything** — never researches or codes directly
- **Two speculators per AC** — minimal vs adversarial lenses ensure divergence
- **Resolver merges** — catches AC ambiguity before implementation
- **GH issues as checkpoints** — work survives context crashes
- **One PR per issue** — each branch independently reviewed and merged
- **Adversarial at every gate** — spec divergence, commit review, PR review

## Go Environment

**CRITICAL:** All Go commands use the toolchain defined in `go.mod`:

```bash
go test ./...              # Run all tests
go test -v ./internal/...  # Verbose, specific packages
go build ./cmd/survey      # Build the binary
gofmt -w .                 # Format all Go source
go vet ./...               # Static analysis
```

No `go install` for project dependencies — the module is self-contained via
`go.mod` + `go.sum`.

## Workflow References

- **BDD TDD workflow:** `~/.claude/CLAUDE.md` — Red → ADR → inner loop → Green → Refactor
- **Commit cadence:** Per CLAUDE.md table — each step has a specific prefix
- **Roborev gate:** After every commit — `roborev wait <sha> && roborev show <sha>`
- **Rodney verification:** Required for UI-touching slices per CLAUDE.md
- **Design principles rubric:** `~/.claude/skills/_shared/design-principles-rubric.md`

## Project Structure

```
pollmd/
├── AGENTS.md                     # This file
├── cmd/survey/main.go            # Entrypoint, env wiring
├── internal/
│   ├── server/server.go          # Routes, vote/result/thanks handlers, bot UA filter
│   ├── server/thanks.html        # Embedded thanks page
│   ├── server/result.html        # Embedded result page
│   ├── server/home.html          # Embedded home page
│   ├── server/landing.html       # Embedded landing page (registered surveys)
│   ├── server/style.css          # Shared stylesheet (served at /style.css)
│   ├── store/store.go            # DuckDB open, schemas, quack_serve, tallies, allowlist
│   └── voter/hash.go             # Daily salt + voter hash
├── deploy/
│   ├── railway/Dockerfile        # Railway image (multi-stage Go build → debian-slim)
│   ├── install-on-server.sh      # Idempotent FreeBSD installer
│   ├── survey.rc                 # FreeBSD rc.d service script
│   └── survey.env.example        # Env-var template
├── docs/                         # Hugo + Hextra site → pollmd.ssp.sh
│   ├── hugo.yaml
│   ├── content/
│   │   └── docs/                 # Usage, architecture, querying, privacy, FAQ, install guides
│   ├── layouts/
│   └── prompts/                  # Initial design specs
├── static/images/                # Screenshots
├── Makefile                      # All dev/deploy/smoke targets
├── CHANGELOG.md
├── README.md
├── go.mod
├── go.sum
├── railway.json
└── .dockerignore
```

## Code Style

- **Language:** Go 1.24
- **Build tool:** `go build` (no Makefile wrappers for local dev; Makefile is for deploy)
- **Types:** Idiomatic Go — structs for data, interfaces for seams at module boundaries
- **Docs:** Go-style comments on exported symbols
- **Naming:** Go conventions — `camelCase` unexported, `PascalCase` exported
- **Format:** `gofmt -w .` on save
- **Vet:** `go vet ./...` before commit
- **Style:** `gosimple` / `staticcheck` compatible; no external linter config yet
- **Tests:** Standard `testing` package + `httptest` for HTTP handlers

## Common Commands

```bash
go test ./...                        # Full test suite
go test -v -run TestHandleSurvey ./internal/server/   # Single test
go build ./cmd/survey                # Build binary
go vet ./...                         # Static analysis
gofmt -l .                           # Check formatting
gofmt -w .                           # Fix formatting
make test                            # go test ./...
make fmt                             # gofmt -w .
make vet                             # go vet ./...
make smoke                           # End-to-end smoke test (needs env vars)
```

## Architecture

### Modules and Responsibilities

| Package | Purpose | Key Types/Funcs |
|---------|---------|-----------------|
| `cmd/survey` | Entrypoint, env wiring, startup | `main()` — parses env, opens DB, starts HTTP + Quack |
| `internal/server` | HTTP handlers, routing, bot UA filter | `handleSurvey`, `handleResult`, `handleThanks`, `handleHome`, `handleLanding`, `handleStyle` |
| `internal/store` | DuckDB connection, schema, queries | `OpenDB`, `RecordVote`, `TallyBySurvey`, `GetAllowedAnswers`, `StartQuackServe` |
| `internal/voter` | Privacy-preserving voter hash | `Hash(ip, ua, salt, surveyID)` → hex string, salt generation and rotation |

### Key Design Decisions

- **Single writer** — HTTP and Quack share one DuckDB connection (`SetMaxOpenConns(1)` is load-bearing)
- **Privacy by construction** — daily salt in memory, never persisted, rotated at UTC midnight
- **Bot filter** — substring match against ~40 User-Agent patterns (no regex per request)
- **HEAD-prefetch tolerance** — Microsoft Safe Links / Gmail prefetchers get 200 without recording a vote
- **Markdown-native** — polls are plain `[Label](URL)` links, no embeds, no JS, no platform lock-in
- **Deploy paths** — Railway (Docker), Linux (binary), FreeBSD (source build + rc.d)
