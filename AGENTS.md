# Repository Guidelines

## Project Structure

- `cmd/verkcli/`: CLI entrypoint.
- `internal/cli/`: Cobra commands and CLI implementation.
- `docs/`: usage notes (`thumbnail.md`, `footage.md`).
- `bin/`: local build output (ignored).
- `.cache/`: local Go build/mod caches used by `make` (ignored).
- `.agents/skills/verkcli/`: Codex skill that teaches agents how to use this CLI.

## Build, Test, And Development Commands

- `make build`: build `bin/verkcli` (uses repo-local `GOCACHE`/`GOMODCACHE` under `.cache/`).
- `make test`: run unit tests.
- `make tidy`: `go mod tidy` with repo-local caches.
- `./bin/verkcli --help`: inspect command surface area; use `<cmd> --help` for subcommands.

## Coding Style & Conventions

- Format with standard Go tooling (`gofmt`); keep changes idiomatic and small.
- CLI output:
  - Keep stdout machine-parseable when `--output json` is selected.
  - Send human hints/progress/errors to stderr (stdout is the data channel).
- Prefer adding typed subcommands over expanding `verkcli request` examples.

## Testing Guidelines

- Add unit tests next to implementation in `internal/cli/*_test.go`.
- Prefer `httptest` and pure functions (URL builders, parsers, formatters) to avoid network dependencies.

## Security & Configuration Tips

- Never commit API keys, bearer tokens, `org_id`, or local config files.
- Prefer env vars for secrets in shells/CI:
  - `VERKADA_API_KEY`, `VERKADA_BASE_URL`, `VERKADA_ORG_ID`, `VERKADA_PROFILE`
- Base URL must be an API host (`https://api.verkada.com` or `.eu`/`.au`), not the web UI host (`*.command.verkada.com`).

## Agent Skill (Codex)

- Skill location (in-repo): `.agents/skills/verkcli/`
- Local install (symlink): `ln -sfn "$PWD/.agents/skills/verkcli" ~/.codex/skills/verkcli`
