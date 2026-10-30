# oryn

A terminal hub for your projects. `oryn` puts what you'd otherwise check across
a dozen tabs on one keyboard-driven screen: what needs you, what's deployed,
what's happening on GitHub, and whether anything you depend on broke. Deploy
and clean up from the same place.

Local-first: no account, no server. Your code never leaves your machine. oryn
talks directly to the services you connect, and sends only package names and
versions to public APIs (OSV, npm, PyPI, Go proxy).

```
 oryn   Home  Projects  Agents  Deploys  GitHub  Health  Activity          16:27

 Needs you                               Deploys

 ▸ devdoq   ● CI is failing on main        ● portfolio   cloudflare 2h
   devdoq   ● 9 issues are assigned to you
            ● 4 PRs are waiting for review
                                         GitHub

                                           devdoq-workers#28   Observability
                                           RX-website#10       Add files via upload

 tab switch · enter open · 1-7 tabs · ? help
```

## Install

```sh
make install              # → ~/.local/bin/oryn
```

Tagged releases publish archives for Linux, macOS and Windows with a
`SHA256SUMS` file. Once the repo is public they also carry GitHub build-provenance attestations:

```sh
sha256sum -c --ignore-missing SHA256SUMS
gh attestation verify oryn_<version>_linux_amd64.tar.gz --repo Arnav717/oryn
```

## Use

```sh
oryn                 # open the terminal
oryn add             # watch the current directory (once per project)
oryn daemon          # background refresh + desktop alerts
oryn ls · rm · scan · version
```

`oryn` opens on Home, with a tab bar that stays on screen everywhere. Keys `1`
to `7` switch tabs from any screen, `esc` goes back, and `ctrl+p` opens a command
palette with every action and project. `?` lists every key.

| Key | Tab      | Shows                                                       |
|-----|----------|-------------------------------------------------------------|
| `1` | Home     | Four windows: Needs you, Agents, Deploys, GitHub            |
| `2` | Projects | Every watched project; `enter` opens one, `a` adds, `d` removes |
| `3` | Agents   | Every AI agent on this machine: who is waiting on you, what is running |
| `4` | Deploys  | Every deployment on every platform                          |
| `5` | GitHub   | Your inbox: PRs to review, your own PRs                     |
| `6` | Health   | Dependencies, vulnerabilities, outdated packages, upstream status for all projects |
| `7` | Activity | Timeline of everything: what's new since you last looked, then earlier events |

Everywhere: `j/k` move · `r` refresh · `?` help · `q` quit.

On Home: `tab` or `h/l` switch window · `enter` opens the selected row. **Needs
you** ranks what's waiting on you: failed deploys, failing CI, PRs to review,
assigned issues and critical vulnerabilities that are new since your last
visit. Known vulnerabilities and upstream outages stay on Health.
Windows stack on a narrow terminal, and a very short one shows only the focused
window.

On Activity: `f` filters (all, deploys, github, health), `e` expands
vulnerabilities folded by package, `enter` opens the event.

On Health: `tab` switch pane · `enter` open package · `/` search (`lod`, `chi`;
`tab` completes, `enter` opens).

### Agents

oryn reads what Claude Code, Codex, OpenCode and Antigravity (`agy`) leave on
this machine and shows every session from the last week, most urgent first.
It only reads: it never writes to those tools' files, and nothing leaves your
machine. `enter` shows a session and the command to resume it. A session
waiting on you also lands in Needs you. These are private formats that can
change, so a tool oryn can't read simply doesn't appear. Codex and OpenCode
don't record when they're waiting for approval, so they only show as working
or idle.

`n` starts an agent: pick a project and tool, describe the task. It gets its
own git worktree and branch (`oryn/<task>`) from your last commit, and runs in
tmux so it keeps going after you quit oryn. `a` opens its live terminal and
`ctrl+]` comes back. `x` stops it and keeps its work. Agents keep their normal
approval prompts, and oryn shows an agent as waiting when one is on its screen.
Claude Code asks you to trust each new worktree once: press `a` and confirm. A
worktree separates files, it isn't a sandbox. Gitignored
files such as `.env` and `node_modules` aren't copied in. Needs tmux, and oryn
uses a private tmux server so your own sessions aren't touched.

### Projects are containers

Opening a project gives you its own tabs (`[` and `]` switch): **Overview**,
**Agents**, **Deploys**, **GitHub**, **Health** and **Activity**. It opens on Overview: what needs you
here, where it's live, GitHub state and recent commits. `enter` opens
the detail for a row, `p` deploys, `D` and `g` jump to Deploys and GitHub. The
GitHub tab is read from the folder's git remote (`origin` first); no setup needed. It has eight sections
(`h`/`l`):

| Section  | Shows                                                         |
|----------|---------------------------------------------------------------|
| Overview | stars, forks, default-branch CI, latest release/commit/run, and a **Needs attention** list (failing CI, PRs awaiting your review, issues assigned to you, critical alerts) |
| PRs      | open PRs with CI, review state, author, age                   |
| Issues   | open issues with labels, comments, author, age                |
| Actions  | recent workflow runs and their status                         |
| Commits  | recent commits on the default branch                          |
| People   | collaborators with their role, plus contributors with commit counts |
| Releases | recent releases                                               |
| Alerts   | open Dependabot alerts (needs admin access to the repo)       |

`enter` opens the selected row in your browser, `o` opens the repo.

If a folder's remote isn't GitHub (or you want a different repo), map it:

```toml
[repos]
"~/code/app" = "owner/name"
```

### GitHub inbox

The **GitHub** tab lists PRs waiting on
your review and your own open PRs across every repo, with CI state, review
state and age. Your own PRs are sorted so failing CI and requested changes
come first. `enter` opens the PR in your browser.

oryn reads your login from the GitHub CLI (`gh auth login`) or `GH_TOKEN` /
`GITHUB_TOKEN`. It stores nothing, and the token is only sent to
`api.github.com`. Set `github = false` in the config to turn it off.

### Background alerts

`oryn daemon` re-scans on the refresh interval and sends a desktop
notification for new high/critical vulnerabilities, upstream status changes,
new review requests, failing CI, and new issues or PRs from other people on
your watched repos. A repo you've just added is a silent baseline, so its
existing issues don't all fire at once (`notify-send` on Linux, `osascript` on macOS). The first run records
a silent baseline so you aren't flooded with everything that already exists.
Run it at login with the systemd unit in `contrib/oryn.service`.

## What it watches

| Pane      | Source                                             |
|-----------|----------------------------------------------------|
| Packages  | `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `go.mod`, `Cargo.lock`, `uv.lock`, `poetry.lock`, `requirements*.txt` |
| Vulns     | [OSV.dev](https://osv.dev) (GHSA, Go, PyPI, RustSec advisories) |
| Latest    | npm registry, PyPI, Go module proxy, crates.io (direct deps) |
| Upstream  | statuspage `/api/v2/status.json` endpoints: your configured services plus vendors detected in your lockfiles (Sentry, Twilio, MongoDB, ...). Turn the detected ones off with `stack_status = false` |
| GitHub    | GraphQL + REST API: review requests, your PRs, and per-repo PRs, issues, Actions, commits, people, releases, Dependabot alerts |
| Deploys   | Vercel, Netlify and Cloudflare Pages: detected from `vercel.json`, `.vercel/`, `netlify.toml`, `wrangler.*`; connect an account in the TUI (`A`: paste a token, oryn verifies it before saving), or use `VERCEL_TOKEN` / `NETLIFY_AUTH_TOKEN` / `CLOUDFLARE_API_TOKEN` + `CLOUDFLARE_ACCOUNT_ID`, or the platform CLI login. Pick a project for a folder with enter on the Deploys tab, or create a brand-new one from the same picker (`+ Create a new project`) for a first deploy. Press `p` to deploy a linked project: oryn shows the exact platform-CLI command (vercel, netlify or wrangler) and runs it only after you confirm. Links are kept in your oryn config, never written into the project. Failed deploys land on Activity |
| Activity  | every new vuln, release, status change, failed deploy and GitHub event, stored in SQLite |

Direct vs. transitive comes from the lockfile where it records it (npm, pnpm,
Cargo, uv) and from the sibling `package.json` / `pyproject.toml` otherwise
(yarn, poetry).

Config: `~/.config/oryn/config.toml` (see `config.example.toml`).
Data: `~/.local/share/oryn/oryn.db`.

## Layout

```
cmd/oryn/          entrypoint: tui, scan, daemon
internal/browser/  open URLs
internal/gitrepo/  folder → GitHub repo
internal/notify/   desktop notifications
internal/config/   config loading
internal/detect/   lockfile parsers
internal/source/   OSV, registries, status pages
internal/store/    SQLite activity timeline
internal/app/      refresh: detect → sources → events
internal/tui/      Bubble Tea UI
```

## Roadmap

- Agents: review their diffs and open PRs from oryn, and hand them a failing build
- `oryn context` and an MCP server, so agents get a project's state without wandering
- AI features: daily brief, explain a failed deploy or CI run, changelog impact
- rollback and promote, live deploy logs
- later: upgrade desk, custom layouts, exec-based custom sources, team sync
