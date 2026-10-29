# oryn

A Bloomberg-style terminal for developers. `oryn` watches everything that
affects your code (vulnerabilities in your dependencies, new releases,
upstream outages) and puts it on one keyboard-driven screen.

Local-first: no account, no server. Your code never leaves your machine; only
package names and versions are sent to public APIs (OSV, npm, PyPI, Go proxy).

```
 ORYN  640 pkgs · 130 vulns (8 crit, 42 high) · 35 outdated · 1 upstream issue(s)
╭ 1 PORTFOLIO ──────────────────────────╮╭ 2 VULNS ─────────────────────────────╮
│ ↳ golang.org/x/crypto  v0.32.0  -  ▲37 ││ CRIT golang.org/x/crypto  CVE-2026-… │
│   vite                 6.4.1  8.3.5 ▲26 ││ CRIT protobufjs@6.11.5    CVE-2026-… │
╰────────────────────────────────────────╯╰──────────────────────────────────────╯
╭ 3 UPSTREAM ─────────────╮╭ 4 TAPE ────────────────────────────────────────────╮
│ ● Cloudflare  Partial…  ││ Oct29 15:29 STATUS  Cloudflare: Partial System Outage│
╰─────────────────────────╯╰────────────────────────────────────────────────────╯
```

## Install

```sh
make install              # → ~/.local/bin/oryn
```

## Use

```sh
cd ~/code/myapp && oryn add    # watch a project (once)
oryn                           # open the terminal
oryn ls · oryn rm · oryn scan
```

With nothing watched, `oryn` uses the current directory.

Keys: `tab` switch pane · `j/k` move · `enter` open · `/` search ·
`r` refresh · `?` help · `q` quit.

Search: press `/`, type a few letters (`lod`, `chi`). Matches appear live;
`tab` completes, `enter` opens. Single letters jump: `p` portfolio, `v` vulns,
`s` status, `t` tape.

## What it watches

| Pane      | Source                                             |
|-----------|----------------------------------------------------|
| Portfolio | `package-lock.json`, `go.mod`, `requirements*.txt` |
| Vulns     | [OSV.dev](https://osv.dev) (GHSA, Go, PyPI advisories) |
| Latest    | npm registry, PyPI, Go module proxy (direct deps)  |
| Upstream  | statuspage `/api/v2/status.json` endpoints         |
| Tape      | every new vuln, release and status change, stored in SQLite |

Config: `~/.config/oryn/config.toml` (see `config.example.toml`).
Data: `~/.local/share/oryn/oryn.db`.

## Layout

```
cmd/oryn/          entrypoint: tui, scan
internal/config/   config loading
internal/detect/   lockfile parsers
internal/source/   OSV, registries, status pages
internal/store/    SQLite event tape
internal/app/      refresh: detect → sources → events
internal/tui/      Bubble Tea UI
```

## Roadmap

- v0.2: `oryn daemon` + desktop notifications, alert rules, custom layouts,
  exec-based custom sources/panes, pnpm/yarn/poetry/uv/Cargo lockfiles
- v0.3: GitHub PR/CI queue, LLM + cloud cost desk, upgrade desk
- v0.4: AI changelog impact analysis, daily brief
