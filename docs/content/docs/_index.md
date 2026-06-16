---
title: Overview
weight: 0
---

A minimal Go service that records anonymous newsletter reader ratings into a
single [DuckDB](https://ssp.sh/brain/duckdb) file. Per-newsletter,
per-answer, no cookies, no JS. Query the results from your laptop over Quack.

The whole thing is around 200 lines of Go and one DuckDB file. Read the
source in an afternoon, fork it if you want different behaviour.

## Why pollmd?

{{< readme-section start="## Why pollmd?" end="## Features" >}}

## Features

{{< readme-section start="## Features" end="## What it looks like in a newsletter" >}}

## Next

- [Install](./install/) — pick a deploy target (Railway, Linux, FreeBSD).
- [Usage](./usage/) — write the markdown links, optionally lock answers per newsletter.
- [Architecture](./architecture/) — how HTTP and Quack share one Go process.
