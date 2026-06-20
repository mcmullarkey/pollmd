HOST       ?= ti
SSH        := ssh $(HOST)
SRC_DIR    := /home/sspaeti/survey-src
BIN        := /usr/local/bin/survey
DUCKDB_VER ?= 1.5.3

# duckdb_use_lib dynamically links against the system libduckdb (1.5.3) on the
# FreeBSD host, since duckdb-go's v2 bindings ship pre-built libs for darwin/
# linux/windows only.
BUILD_FLAGS := -tags=duckdb_use_lib

.PHONY: test fmt vet sync build deploy logs status query token duckdb-connect push-installer install-on-server smoke help \
        railway-token railway-docker-build railway-docker-run railway-duckdb-connect survey-result survey-reset survey-create survey-delete \
        docs docs-serve docs-clean

# Public HTTPS host for the Railway deploy (newsletter readers click here).
# Railway terminates TLS on 443 and proxies to the container's port 8080
# internally — externally there is no port in the URL. Override for a
# self-hosted deploy: `make smoke SURVEY_HOST=survey.example.org`.
SURVEY_HOST ?= q.ssp.sh

# Quack endpoint — kept out of the repo because publishing the admin endpoint
# alongside a leaked token would skip the "find the endpoint" step. The token
# itself is the real lock, but defence-in-depth.
#
# Defaults to whatever RAILWAY_QUACK_HOST / RAILWAY_QUACK_PORT are exported in
# the shell (the same env vars `survey-result` and `railway-duckdb-connect`
# already consume), so a single direnv / shell profile feeds every Quack-using
# target. Note: survey-create/reset/delete now use the HTTP API via pollctl
# instead of Quack. Override on the CLI for self-hosted:
# `make smoke QUACK_HOST=quack.example.org QUACK_PORT=9494`.
QUACK_HOST  ?= $(RAILWAY_QUACK_HOST)
QUACK_PORT  ?= $(RAILWAY_QUACK_PORT)

# Public base URL printed by survey-create — the landing page URL plus
# the markdown answer links are formatted using this. Override on the
# CLI if you serve under a different host.
PUBLIC_URL  ?= https://q.ssp.sh

# Admin HTTP API (used by pollctl for survey management).
# Point to a running server instance (local or remote).
POLLMD_ADMIN_URL ?= http://127.0.0.1:8080
POLLMD_ADMIN_TOKEN ?= $(SURVEY_ADMIN_TOKEN)
export SURVEY_ADMIN_URL := $(POLLMD_ADMIN_URL)
export SURVEY_ADMIN_TOKEN := $(POLLMD_ADMIN_TOKEN)

help:
	@echo "Laptop targets (run on Arch):"
	@echo "  push-installer    - rsync source + installer to $(HOST):$(SRC_DIR)/,"
	@echo "                      then print the next-steps for the manual root install."
	@echo "  sync              - rsync source to $(HOST)"
	@echo "  build             - sync, then build Go binary on $(HOST)"
	@echo "  deploy            - build, then atomically swap binary and restart service"
	@echo "  logs              - tail service log on $(HOST)"
	@echo "  status            - show service status on $(HOST)"
	@echo "  query             - print the Quack ATTACH snippet to paste into duckdb"
	@echo "  token             - print the Quack token (reads from \$$HOST as root)"
	@echo "  duckdb-connect    - open local duckdb with the remote DB attached as 's'"
	@echo "                      (needs \$$SURVEY_QUACK_TOKEN env var)"
	@echo "  smoke             - end-to-end check (DNS + TLS + /healthz + HEAD /survey)"
	@echo "  test / fmt / vet  - local Go targets"
	@echo ""
	@echo "Docs site targets (pollmd.ssp.sh, Hugo + Hextra in docs/):"
	@echo "  docs              - one-shot Hugo build into docs/public/"
	@echo "  docs-serve        - run the Hugo dev server (localhost:1310)"
	@echo "  docs-clean        - delete docs/public, docs/resources, .hugo_build.lock"
	@echo ""
	@echo "Railway targets (see docs/content/docs/install/railway.md):"
	@echo "  railway-token            - print a fresh SURVEY_QUACK_TOKEN to paste into Railway env"
	@echo "  railway-docker-build     - docker build the Railway image locally for testing"
	@echo "  railway-docker-run       - docker run the image with a tmp volume + generated token"
	@echo "  railway-duckdb-connect   - duckdb attached to Railway's TCP Proxy"
	@echo "                             (needs RAILWAY_QUACK_HOST, RAILWAY_QUACK_PORT,"
	@echo "                              SURVEY_QUACK_TOKEN env vars)"
	@echo "  survey-result            - one-shot tally with bar chart of all surveys"
	@echo "                             (uses Quack, needs SURVEY_QUACK_TOKEN + QUACK_HOST/"
	@echo "                              QUACK_PORT)"
	@echo "                             pass SURVEY_ID=<id> for a single-survey detail view"
	@echo "  survey-reset             - Clear all votes for SURVEY_ID (keeps registration)."
	@echo "                             Uses HTTP API via pollctl. Prompts to confirm"
	@echo "                             (skip with CONFIRM=yes)."
	@echo "                             usage: make survey-reset SURVEY_ID=<id>"
	@echo "  survey-create            - Register a SURVEY_ID with allowed answers."
	@echo "                             Uses HTTP API via pollctl. Requires a running"
	@echo "                             server + POLLMD_ADMIN_TOKEN (or SURVEY_ADMIN_TOKEN)."
	@echo "                             Prints landing URL and ready-to-paste markdown links."
	@echo "                             usage: make survey-create SURVEY_ID=<id> ANSWERS=a,b,c"
	@echo "  survey-delete            - Fully remove a survey (votes + registration)."
	@echo "                             Uses HTTP API via pollctl. Prompts to confirm"
	@echo "                             (skip with CONFIRM=yes)."
	@echo "                             usage: make survey-delete SURVEY_ID=<id>"
	@echo ""
	@echo "Server target (run on FreeBSD as root, after 'ssh $(HOST)' && 'su root'):"
	@echo "  install-on-server - one-shot setup: pkg deps, DuckDB $(DUCKDB_VER) build,"
	@echo "                      user/dirs, env+token, rc.d, sudoers. Idempotent."
	@echo "                      TLS terminates on your existing reverse proxy, not"
	@echo "                      Caddy. Add two NPM hosts:"
	@echo "                        survey.sspaeti.duckdns.org -> http://<ti-LAN-ip>:8080"
	@echo "                        quack.sspaeti.duckdns.org  -> http://<ti-LAN-ip>:9494"

push-installer: sync
	@echo ""
	@echo "==> Installer pushed to $(HOST):$(SRC_DIR)/"
	@echo ""
	@echo "Now run, in this order:"
	@echo "  ssh $(HOST)"
	@echo "  su root                            # enter root password"
	@echo "  cd $(SRC_DIR)"
	@echo "  make install-on-server             # ~20 min first time (DuckDB build)"
	@echo "  exit                               # leave root"
	@echo "  exit                               # leave ssh"
	@echo ""
	@echo "Then back here: make deploy"

install-on-server:
	@if [ "$$(id -u)" -ne 0 ]; then \
	    echo "error: install-on-server must run as root on the FreeBSD host." >&2; \
	    echo "       Do: ssh $(HOST), then 'su root', then 'cd $(SRC_DIR) && make install-on-server'." >&2; \
	    exit 1; \
	fi
	DUCKDB_VER=$(DUCKDB_VER) sh deploy/install-on-server.sh

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

sync:
	rsync -az --delete \
	  --exclude build/ --exclude .git/ --exclude docs/ --exclude .DS_Store \
	  ./ $(HOST):$(SRC_DIR)/

build: sync
	$(SSH) "cd $(SRC_DIR) && \
	        CGO_CFLAGS='-I/usr/local/include' \
	        CGO_LDFLAGS='-L/usr/local/lib -lduckdb' \
	        go build $(BUILD_FLAGS) -o build/survey ./cmd/survey"

deploy: build
	$(SSH) "sudo /bin/cp $(SRC_DIR)/build/survey $(BIN).new && \
	        sudo /bin/mv $(BIN).new $(BIN) && \
	        sudo /usr/sbin/service survey restart"

logs:
	$(SSH) "sudo /usr/bin/tail -f /var/log/survey/survey.log"

status:
	$(SSH) "sudo /usr/sbin/service survey status"

query:
	@echo "Paste into duckdb on your laptop:"
	@echo ""
	@echo "  CREATE SECRET (TYPE quack, TOKEN '<token from /usr/local/etc/survey/survey.env>');"
	@echo "  ATTACH 'quack:$(QUACK_HOST):$(QUACK_PORT)' AS s;"
	@echo "  FROM s.votes ORDER BY ts DESC LIMIT 20;"

# Print the Quack token from the server. Use as:
#   export SURVEY_QUACK_TOKEN=$(make -s token)
token:
	@$(SSH) "sudo /usr/bin/cat /usr/local/etc/survey/survey.env" \
	    | awk -F= '/^SURVEY_QUACK_TOKEN=/{sub(/^SURVEY_QUACK_TOKEN=/,""); print}'

# Open duckdb locally with the remote Quack DB already attached as `s`.
# Token comes from the SURVEY_QUACK_TOKEN env var; the SQL goes through an
# mktemp init file (0600) so the token never appears in argv / ps output.
duckdb-connect:
	@command -v duckdb >/dev/null || { echo "error: duckdb CLI not on PATH (install duckdb locally first)" >&2; exit 1; }
	@if [ -z "$$SURVEY_QUACK_TOKEN" ]; then \
	    echo "error: SURVEY_QUACK_TOKEN not set" >&2; \
	    echo "       export SURVEY_QUACK_TOKEN=\$$(make -s token)" >&2; \
	    exit 1; \
	fi
	@tmp=$$(mktemp) && trap "rm -f $$tmp" EXIT INT TERM HUP && \
	  printf "INSTALL quack;\nLOAD quack;\nATTACH 'quack:%s:%s' AS s (TOKEN '%s');\n" \
	    "$(QUACK_HOST)" "$(QUACK_PORT)" "$$SURVEY_QUACK_TOKEN" > "$$tmp" && \
	  echo "" && \
	  echo "Connected. Try:  FROM s.votes ORDER BY ts DESC LIMIT 10;" && \
	  echo "" && \
	  duckdb -init "$$tmp"

# End-to-end smoke test from the laptop. Uses HEAD on /<id>/<answer> (server
# returns 200 without recording — anti-prefetch behavior), so no vote is
# created. The Quack endpoint is checked as raw TCP since Railway's TCP Proxy
# is not HTTPS. Override SURVEY_HOST / QUACK_HOST / QUACK_PORT for non-Railway
# deployments.
#
# NOTE: probe slugs must satisfy ^[a-z0-9][a-z0-9_-]{0,63}$ — otherwise the
# server returns 400 from the slug check and /result/<id> fails even with a
# healthy upstream. "smoke-test" matches; "_smoke" does not.
smoke:
	@if [ -z "$(QUACK_HOST)" ] || [ -z "$(QUACK_PORT)" ]; then \
	    echo "error: QUACK_HOST / QUACK_PORT not set (default reads RAILWAY_QUACK_HOST / RAILWAY_QUACK_PORT from env)" >&2; \
	    echo "       export them in your shell, or pass on the CLI:" >&2; \
	    echo "         make smoke QUACK_HOST=<host> QUACK_PORT=<port>" >&2; \
	    exit 1; \
	fi
	@set -e; \
	pass=0; fail=0; \
	check() { name="$$1"; shift; \
	    if out=$$("$$@" 2>&1); then echo "  PASS  $$name"; pass=$$((pass+1)); \
	    else echo "  FAIL  $$name: $$out"; fail=$$((fail+1)); fi; }; \
	echo "DNS"; \
	check "$(SURVEY_HOST) resolves" sh -c 'dig +short $(SURVEY_HOST) | grep -E "^[0-9]+\."'; \
	check "$(QUACK_HOST) resolves"  sh -c 'dig +short $(QUACK_HOST)  | grep -E "^[0-9]+\."'; \
	echo "HTTPS (Railway edge)"; \
	check "$(SURVEY_HOST) TLS OK"   sh -c 'curl -sf --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/healthz | grep -E "^(200|502|404)$$"'; \
	echo "Quack TCP proxy"; \
	check "$(QUACK_HOST):$(QUACK_PORT) reachable" sh -c 'nc -z -w 5 $(QUACK_HOST) $(QUACK_PORT)'; \
	echo "Survey service"; \
	check "/healthz returns 200"    sh -c '[ "$$(curl -s --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/healthz)" = "200" ]'; \
	check "HEAD /<id>/<answer> returns 200 (no vote recorded)" \
	                                sh -c '[ "$$(curl -sI --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/smoke-test/probe)" = "200" ]'; \
	check "HEAD /survey/<id>/<answer> still 200 (legacy back-compat)" \
	                                sh -c '[ "$$(curl -sI --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/survey/smoke-test/probe)" = "200" ]'; \
	check "/thanks returns 200"     sh -c '[ "$$(curl -s --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/thanks)" = "200" ]'; \
	check "/result/<id> returns 200" sh -c '[ "$$(curl -s --max-time 8 -o /dev/null -w "%{http_code}" https://$(SURVEY_HOST)/result/smoke-test)" = "200" ]'; \
	echo ""; \
	echo "$$pass passed, $$fail failed"; \
	[ "$$fail" = "0" ]

# --- Docs site -----------------------------------------------------------
# Wrappers so you can run `make docs` from the project root rather than
# `cd docs && make build` every time. The real targets live in docs/Makefile;
# these just delegate.

docs:
	@$(MAKE) -C docs build

docs-serve:
	@$(MAKE) -C docs serve

docs-clean:
	@$(MAKE) -C docs clean

# --- Railway -------------------------------------------------------------
# See docs/install-railway.md for the full one-time setup.

RAILWAY_IMAGE ?= survey:railway

railway-token:
	@head -c 32 /dev/urandom | base64 | tr -d '\n'; echo

railway-docker-build:
	docker build -f deploy/railway/Dockerfile -t $(RAILWAY_IMAGE) .

railway-docker-run:
	@token=$$(head -c 32 /dev/urandom | base64 | tr -d '\n'); \
	echo "Generated SURVEY_QUACK_TOKEN (one-shot): $$token"; \
	docker run --rm -it \
	    -p 8080:8080 -p 9494:9494 \
	    -e SURVEY_HTTP_ADDR=0.0.0.0:8080 \
	    -e SURVEY_QUACK_ADDR=0.0.0.0:9494 \
	    -e SURVEY_QUACK_TOKEN="$$token" \
	    -v survey-data:/var/db/survey \
	    $(RAILWAY_IMAGE)

# Open local duckdb with helpers pre-defined for the remote Quack server.
#
# ATTACH 'quack:...' is broken in the quack extension build shipped with
# DuckDB 1.5.3 (Binder Error: Catalog "x" does not exist on a fresh ATTACH).
# quack_query() works fine, so we wrap it in a macro + view:
#
#   FROM remote_votes;                          -- whole table
#   FROM rq('SELECT survey_id, count(*)         -- arbitrary remote SQL
#           FROM votes GROUP BY ALL');
#
# Each call fetches a fresh snapshot from the server. Fine for the vote
# volumes this app handles.
railway-duckdb-connect:
	@command -v duckdb >/dev/null || { echo "error: duckdb CLI not on PATH (install duckdb locally first)" >&2; exit 1; }
	@if [ -z "$$SURVEY_QUACK_TOKEN" ] || [ -z "$$RAILWAY_QUACK_HOST" ] || [ -z "$$RAILWAY_QUACK_PORT" ]; then \
	    echo "error: need SURVEY_QUACK_TOKEN, RAILWAY_QUACK_HOST, RAILWAY_QUACK_PORT" >&2; \
	    echo "       see docs/install-railway.md" >&2; \
	    exit 1; \
	fi
	@tmp=$$(mktemp) && trap "rm -f $$tmp" EXIT INT TERM HUP && \
	  printf "INSTALL quack;\nLOAD quack;\nCREATE OR REPLACE MACRO rq(sql) AS TABLE (FROM quack_query('quack:%s:%s', sql, token => '%s', disable_ssl => true));\nCREATE OR REPLACE VIEW remote_votes AS FROM rq('FROM votes');\n" \
	    "$$RAILWAY_QUACK_HOST" "$$RAILWAY_QUACK_PORT" "$$SURVEY_QUACK_TOKEN" > "$$tmp" && \
	  echo "" && \
	  echo "Connected via quack_query (ATTACH is broken in quack 1.5.3)." && \
	  echo "Try:" && \
	  echo "  FROM remote_votes;" && \
	  echo "  FROM remote_votes WHERE survey_id = '2026-06-04';" && \
	  echo "  FROM rq('SELECT survey_id, answer, count(*) FROM votes GROUP BY ALL');" && \
	  echo "" && \
	  duckdb -init "$$tmp"

# Fetch a remote tally via Quack SQL (read-only, still requires Quack).
# Shows an ASCII bar chart for SURVEY_ID, or all surveys if SURVEY_ID is empty.
#
#   make survey-result SURVEY_ID=my-poll
#   make survey-result                          # all surveys
#
# Requires duckdb CLI + SURVEY_QUACK_TOKEN, RAILWAY_QUACK_HOST,
# RAILWAY_QUACK_PORT. Unlike survey-create/reset/delete, this target
# queries the remote database directly via Quack rather than the HTTP API.
SURVEY_ID ?=

survey-result:
	@command -v duckdb >/dev/null || { echo "error: duckdb CLI not on PATH (install duckdb locally first)" >&2; exit 1; }
	@if [ -z "$$SURVEY_QUACK_TOKEN" ] || [ -z "$$RAILWAY_QUACK_HOST" ] || [ -z "$$RAILWAY_QUACK_PORT" ]; then \
	    echo "error: need SURVEY_QUACK_TOKEN, RAILWAY_QUACK_HOST, RAILWAY_QUACK_PORT" >&2; \
	    echo "       see docs/install-railway.md" >&2; \
	    exit 1; \
	fi
	@if [ -n "$(SURVEY_ID)" ] && ! echo "$(SURVEY_ID)" | grep -qE '^[a-z0-9][a-z0-9_-]{0,63}$$'; then \
	    echo "error: SURVEY_ID must match ^[a-z0-9][a-z0-9_-]{0,63}\$$" >&2; \
	    exit 1; \
	fi
	@tmp=$$(mktemp) && trap "rm -f $$tmp" EXIT INT TERM HUP && \
	  printf "INSTALL quack;\nLOAD quack;\nCREATE OR REPLACE MACRO rq(sql) AS TABLE (FROM quack_query('quack:%s:%s', sql, token => '%s', disable_ssl => true));\nCREATE OR REPLACE VIEW remote_votes AS FROM rq('FROM votes');\n" \
	    "$$RAILWAY_QUACK_HOST" "$$RAILWAY_QUACK_PORT" "$$SURVEY_QUACK_TOKEN" > "$$tmp" && \
	  if [ -n "$(SURVEY_ID)" ]; then \
	    duckdb -init "$$tmp" -c "WITH t AS (SELECT answer, count(*) AS clicks FROM remote_votes WHERE survey_id = '$(SURVEY_ID)' GROUP BY answer) SELECT '$(SURVEY_ID)' AS survey_id, answer, clicks, bar(clicks, 0, (SELECT max(clicks) FROM t), 30) AS chart FROM t ORDER BY clicks DESC;"; \
	  else \
	    duckdb -init "$$tmp" -c "WITH t AS (SELECT survey_id, answer, count(*) AS clicks FROM remote_votes GROUP BY ALL), m AS (SELECT survey_id, max(clicks) AS top FROM t GROUP BY survey_id) SELECT t.survey_id, t.answer, t.clicks, bar(t.clicks, 0, m.top, 30) AS chart FROM t JOIN m USING (survey_id) ORDER BY t.survey_id DESC, t.clicks DESC;"; \
	  fi

# Reset all votes for SURVEY_ID via the admin HTTP API.
# Keeps the survey registration (allowed_answers) intact.
#
#   make survey-reset SURVEY_ID=init
#   make survey-reset SURVEY_ID=init CONFIRM=yes    # skip prompt
survey-reset:
	@if [ -z "$(SURVEY_ID)" ]; then \
	    echo "error: SURVEY_ID is required (no default to prevent accidents)" >&2; \
	    echo "       usage: make survey-reset SURVEY_ID=<id>" >&2; \
	    exit 1; \
	fi
	@if ! echo "$(SURVEY_ID)" | grep -qE '^[a-z0-9][a-z0-9_-]{0,63}$$'; then \
	    echo "error: SURVEY_ID must match ^[a-z0-9][a-z0-9_-]{0,63}\$$" >&2; \
	    exit 1; \
	fi
	@if [ "$(CONFIRM)" = "yes" ]; then \
	    go run ./cmd/pollctl reset "$(SURVEY_ID)" --yes; \
	else \
	    go run ./cmd/pollctl reset "$(SURVEY_ID)"; \
	fi

# Register a survey's allowed answer slugs via the admin HTTP API.
#
#   make survey-create SURVEY_ID=2026-06-15 ANSWERS=awesome,good,better,worse
#   make survey-create SURVEY_ID=my-poll ANSWERS=a,b,c MODE=named
#
# Requires a running server and POLLMD_ADMIN_TOKEN (or SURVEY_ADMIN_TOKEN).
# Optional MODE flag: MODE=named|anonymous (default anonymous).
survey-create:
	@if [ -z "$(SURVEY_ID)" ]; then \
	    echo "error: SURVEY_ID is required" >&2; \
	    echo "       usage: make survey-create SURVEY_ID=<id> ANSWERS=a,b,c [MODE=named]" >&2; \
	    exit 1; \
	fi
	@if [ -z "$(ANSWERS)" ]; then \
	    echo "error: ANSWERS is required" >&2; \
	    exit 1; \
	fi
	@if ! echo "$(SURVEY_ID)" | grep -qE '^[a-z0-9][a-z0-9_-]{0,63}$$'; then \
	    echo "error: SURVEY_ID must match ^[a-z0-9][a-z0-9_-]{0,63}\$$" >&2; \
	    exit 1; \
	fi
	@if [ -n "$(MODE)" ] && ! echo "$(MODE)" | grep -qE '^(anonymous|named)$$'; then \
	    echo "error: MODE must be 'anonymous' or 'named'" >&2; \
	    exit 1; \
	fi
	@for a in $$(echo "$(ANSWERS)" | tr ',' ' '); do \
	    echo "$$a" | grep -qE '^[a-z0-9][a-z0-9_-]{0,63}$$' || { \
	        echo "error: invalid answer slug: $$a" >&2; \
	        exit 1; \
	    }; \
	done
	@mode_flag=; if [ -n "$(MODE)" ]; then mode_flag="--mode $(MODE)"; fi; \
	echo "Registering survey_id='$(SURVEY_ID)' with allowed answers: $(ANSWERS)"; \
	go run ./cmd/pollctl create "$(SURVEY_ID)" --answers "$(ANSWERS)" $$mode_flag; \
	echo ""; \
	echo "Landing page (share this URL):"; \
	echo "  $(PUBLIC_URL)/$(SURVEY_ID)"; \
	echo ""; \
	echo "Markdown links to paste into your newsletter:"; \
	echo ""; \
	for a in $$(echo "$(ANSWERS)" | tr ',' ' '); do \
	    label=$$(echo "$$a" | sed 's/-/ /g' | awk '{for(i=1;i<=NF;i++) $$i = toupper(substr($$i,1,1)) substr($$i,2); print}'); \
	    echo "  [$$label]($(PUBLIC_URL)/$(SURVEY_ID)/$$a)"; \
	done; \
	echo ""; \
	echo "Live tally page:"; \
	echo "  $(PUBLIC_URL)/result/$(SURVEY_ID)"; \
	echo ""; \
	echo "done."

# Fully remove a survey (votes + registration) via the admin HTTP API.
#
#   make survey-delete SURVEY_ID=test123
#   make survey-delete SURVEY_ID=test123 CONFIRM=yes    # skip prompt
survey-delete:
	@if [ -z "$(SURVEY_ID)" ]; then \
	    echo "error: SURVEY_ID is required (no default to prevent accidents)" >&2; \
	    echo "       usage: make survey-delete SURVEY_ID=<id>" >&2; \
	    exit 1; \
	fi
	@if ! echo "$(SURVEY_ID)" | grep -qE '^[a-z0-9][a-z0-9_-]{0,63}$$'; then \
	    echo "error: SURVEY_ID must match ^[a-z0-9][a-z0-9_-]{0,63}\$$" >&2; \
	    exit 1; \
	fi
	@if [ "$(CONFIRM)" = "yes" ]; then \
	    go run ./cmd/pollctl delete "$(SURVEY_ID)" --yes; \
	else \
	    go run ./cmd/pollctl delete "$(SURVEY_ID)"; \
	fi
