---
title: Install
weight: 10
---

Pick a deploy target. Each guide is standalone — read just the one for the
platform you actually run on.

{{< cards cols="3" >}}
  {{< card link="./railway/" title="Railway"
    subtitle="Docker image, persistent volume, Quack on TCP Proxy. Easiest path; drives the `railway-*` Makefile targets." >}}
  {{< card link="./linux/" title="Linux (EC2 / Hetzner / anywhere)"
    subtitle="Prebuilt `libduckdb` in the Go bindings, ~10 lines of shell + systemd. Fits any cloud or VPS." >}}
  {{< card link="./freebsd/" title="FreeBSD"
    subtitle="What I run on `ti`. From-source DuckDB build (~20 min) because upstream ships no FreeBSD binaries." >}}
{{< /cards >}}
