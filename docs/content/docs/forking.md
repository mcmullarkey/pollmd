---
title: Fork & Customize
weight: 80
---

This repo is **my deploy** — it ships pre-wired for [ssp.sh](https://www.ssp.sh) (my logo, my colors, my domains `q.ssp.sh` / `pollmd.ssp.sh`, my Twitter handle). pollmd has no config file, no setup script, no theming layer. To make it yours, fork it and grep-and-replace.

```sh
# Fork sspaeti/pollmd via the GitHub UI, then:
git clone git@github.com:<your-username>/pollmd.git
cd pollmd
git remote add upstream https://github.com/sspaeti/pollmd.git
```

## What to change, roughly

As of today the customization touch points are:

- **Branding** — `internal/server/style.css` (five hex values at the top), the logo `<img src="…">` in the four embedded templates under `internal/server/`, and `internal/server/ogimage.png` (the 1200×630 social card).
- **Domains** — search for `q.ssp.sh`, `pollmd.ssp.sh`, `sspaeti/pollmd`, `@sspaeti`, and `ssp.sh/brain/…` across the repo. Most are in `Makefile`, `README.md`, `docs/hugo.yaml`, `docs/static/CNAME`, and the `internal/server/*.html` templates.
- **Docs content** — `docs/content/_index.md` (hero, feature cards) and `docs/content/docs/_index.md` (overview prose). The README pull-throughs handle the technical sections automatically, so leave those wrappers alone.

Use `git grep sspaeti` and `git grep ssp.sh` from the repo root to spot anything this list misses — pollmd is a small repo and a grep covers every reference in a few seconds.

> This list is approximate and will go stale as the repo evolves. If something looks branded that isn't called out here, treat the grep as source of truth, not the page you're reading.

## Pulling upstream changes after you customize

```sh
git fetch upstream
git merge upstream/main           # or: git rebase upstream/main
```

Conflicts land mostly in the files you customized (`style.css`, the templates, `hugo.yaml`, `docs/content/_index.md`). Keep yours, take upstream's structural changes (new fields, new pages, new routes). For the binary `ogimage.png`, `git checkout --ours internal/server/ogimage.png` after a merge if you don't want upstream's image.

## Deploy

Pick a [install target](/docs/install/) once your fork builds and `make test` passes.

If something breaks that the upstream README claims should work, open an issue at [`github.com/sspaeti/pollmd/issues`](https://github.com/sspaeti/pollmd/issues).
