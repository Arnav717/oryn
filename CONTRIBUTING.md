# Contributing to oryn

Thanks for helping. Bug reports, ideas and pull requests are all welcome. For
anything larger than a small fix, open an issue first so we can agree on the
approach before you spend time on it.

## Set up

You need Go (see `go.mod` for the version) and git. tmux is only needed to run
agents and their tests, which skip without it.

```sh
git clone https://github.com/Arnav717/oryn
cd oryn
make test      # run the tests
make check     # what CI runs: format, vet, tests
make build     # builds bin/oryn
```

Try your build without touching your real data:

```sh
mkdir -p /tmp/oryn-dev
./bin/oryn -config /tmp/oryn-dev/config.toml -db /tmp/oryn-dev/oryn.db -p ~/code/some-project
```

`-p` is a one-off view: nothing you do with it changes the saved project list.
A `-db` in another folder keeps oryn's cache, agents and worktrees there too.

## How the code is laid out

```
cmd/oryn/          commands: tui, scan, daemon, context, mcp, claim
internal/tui/      Bubble Tea UI
internal/app/      a refresh: detect, fetch from sources, record events
internal/source/   OSV, registries, status pages, GitHub
internal/deploy/   Vercel, Netlify, Cloudflare Pages
internal/agents/   reads agent sessions; starts them in worktrees under tmux
internal/brief/    the project summary behind `oryn context` and MCP
internal/mcp/      the MCP server
internal/detect/   lockfile parsers
internal/gitrepo/  folder to GitHub repo, and finding repos in a folder
internal/store/    SQLite: activity events and file claims
internal/config/   config loading and saving
```

## Common changes

- **A new top-level tab:** add a `sectionID` and a row in `sections` in
  `internal/tui/sections.go`. Give it an `attention` func to feed Needs you.
- **A new deploy platform:** implement `Provider` in `internal/deploy`, plus
  `Connector`, `Creator` and `Remover` if it supports them.
- **A new agent tool:** add an adapter in `internal/agents` and a row in `Tools`.
  Build the adapter from real files on your machine, and fail quietly: these
  are private formats, and a tool oryn can't read just doesn't appear.
- **A new lockfile format:** add a parser in `internal/detect` with a test.
- **A new MCP tool:** add it to `mcpTools` in `cmd/oryn/context.go`.

## Rules that protect users

These aren't style preferences. A PR that breaks one won't be merged.

- **Ask before anything outward or destructive.** Deploying, deleting, pushing
  and opening PRs all need a confirmation in the TUI. Deleting a project or an
  agent's worktree means typing its name.
- **Never skip an agent's approvals.** `Tools` must not gain a flag like
  `--dangerously-skip-permissions`, and oryn doesn't answer prompts for you.
- **Outsiders' text is data, never instructions.** PR titles, issues, logs and
  deploy messages stay out of `oryn context`, MCP and "Fix this" tasks. Anything
  agent-written shown in the TUI gets terminal control codes stripped.
- **No shell.** Run commands as argument lists with `exec.Command`. User and
  agent text must never reach a shell.
- **MCP stays harmless.** No tool may deploy, delete, push or start anything.
- **Secrets stay out.** Tokens never appear in errors, logs or the UI, and
  credential files are `0600`. oryn never reads `.env` contents.
- **Local-first.** No telemetry, and no server of ours.

## Pull requests

- Keep them focused: one change, with tests for the behaviour it adds or fixes.
- Run `make check` first. CI also builds for Linux, macOS and Windows.
- Match the code around you. Comments explain why, not what.
- For UI changes, add a screenshot, and check a narrow terminal (60x14). `make screenshots` renders the README images from made-up demo data, so no real project names end up in them.
- Follow the look: calm and minimal, one teal accent, no counts or decoration
  that don't earn their place.
- Commit subjects use [Conventional Commits](https://www.conventionalcommits.org):
  `feat:`, `fix:`, `perf:`, `docs:`, `test:`.

By contributing, you agree your work is licensed under the [MIT License](LICENSE).
