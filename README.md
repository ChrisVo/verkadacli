## 📷 verkcli — Inspect cameras, fetch thumbnails, and script Verkada APIs

`verkcli` is a Go CLI for Verkada APIs. It supports named profiles (multiple orgs/regions), typed camera commands, local camera labels, and a raw `request` escape hatch.

This is not an official Verkada project.

## Features

- **Profiles**: keep multiple configs (regions/orgs) and switch with `--profile`.
- **Auto API token**: if an endpoint requires `x-verkada-auth`, the CLI will `POST /token` using your `x-api-key`, cache it, and retry once.
- **Cameras**:
  - `cameras list` (paged, `--all`, `--wide`, filters)
  - `cameras get <camera_id>`
  - `cameras thumbnail` (low-res/hi-res) and optional **inline terminal view** (iTerm2)
- **Local labels**: store friendly names locally (per profile) without modifying anything in Verkada.
- **Scriptable output**: `--output text|json`
- **Raw HTTP**: `verkcli request ...` for endpoints we haven’t typed yet.

## Requirements

- Go (for building from source).
- Network access to the Verkada API.
- `org_id` (required for streaming / footage endpoints like `cameras footage ...`).

Important:
- Your API base URL should be an API host like `https://api.verkada.com` (or `https://api.eu.verkada.com`, `https://api.au.verkada.com`).
- Do not use `*.command.verkada.com` (that is the web UI; you’ll get HTML).

## Install / build

## Homebrew

If you have Homebrew installed:

```bash
brew install ChrisVo/tap/verkcli
verkcli --help
```

(Formula name is `verkcli`, installed binary is `verkcli`.)

Build a local binary:

```bash
mkdir -p bin
go build -o bin/verkcli ./cmd/verkcli
./bin/verkcli --help
```

If you’re in an environment where Go can’t write to the default build cache, use:

```bash
make build
make test
```

## Quick start

Create a config:

```bash
./bin/verkcli config init
```

Login (saves API key and base URL under a profile):

```bash
./bin/verkcli login
```

Or non-interactive:

```bash
./bin/verkcli --profile default login --no-prompt \
  --base-url https://api.verkada.com \
  --org-id "YOUR_ORG_ID" \
  --api-key "$VERKCLI_API_KEY"
```

By default, `login` runs a preflight check (it verifies the API key can list cameras and that the streaming `.m3u8` endpoint works for your `org_id`).
Skip this if you only want to write config:

```bash
./bin/verkcli login --no-verify ...
```

List cameras (text table):

```bash
./bin/verkcli cameras list
./bin/verkcli cameras list --wide
./bin/verkcli cameras list --all
./bin/verkcli cameras list --all --q lobby
./bin/verkcli cameras list --camera-id <camera_id>
```

Get one camera:

```bash
./bin/verkcli cameras get <camera_id>
./bin/verkcli --output json cameras get <camera_id>
```

Fetch a thumbnail:

```bash
./bin/verkcli cameras thumbnail --camera-id <camera_id>
./bin/verkcli cameras thumbnail --camera-id <camera_id> --resolution hi-res --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id <camera_id> --timestamp 1736893300 --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-15T14:30:00Z --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13T08:00:00 --tz America/Los_Angeles --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13 08:00:00 --tz America/Los_Angeles
./bin/verkcli cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13T08:00:00Z --tz America/Los_Angeles
  -> `--tz` is ignored when --timestamp includes an explicit offset like `Z` or `-07:00`.

# Inline render in iTerm2/WezTerm (use --out if you also want a file)
./bin/verkcli cameras thumbnail --camera-id <camera_id> --view
./bin/verkcli cameras thumbnail --camera-id <camera_id> --view --out thumb.jpg
```

See also: [docs/thumbnail.md](docs/thumbnail.md) for full timestamp and timezone rules.

## Profiles

Pick a profile per command:

```bash
./bin/verkcli --profile eu cameras list
```

Set a default profile:

```bash
./bin/verkcli config profiles list
./bin/verkcli config use eu
```

## Local camera labels

Labels are stored locally in your config profile and show up in `cameras list` output.

```bash
./bin/verkcli cameras label set <camera_id> "Front Door"
./bin/verkcli cameras label list
./bin/verkcli cameras label rm <camera_id>
```

Filter using labels:

```bash
./bin/verkcli cameras list --all --q "front"
```

## Raw requests

Use typed commands when available; otherwise:

```bash
./bin/verkcli request --method GET --path /cameras/v1/devices
./bin/verkcli request -H "x-api-key: $VERKCLI_API_KEY" --method POST --path /token
```

## Config

Config defaults to `$XDG_CONFIG_HOME/verkcli/config.json` (often `~/.config/verkcli/config.json`). If you already have a legacy config at `$XDG_CONFIG_HOME/verkada/config.json`, the CLI will use it.

Supported env vars:

- `VERKCLI_PROFILE` (legacy: `VERKADA_PROFILE`)
- `VERKCLI_BASE_URL` (legacy: `VERKADA_BASE_URL`)
- `VERKCLI_ORG_ID` (legacy: `VERKADA_ORG_ID`)
- `VERKCLI_API_KEY` (legacy: `VERKADA_API_KEY`)
- `VERKCLI_TOKEN` (legacy: `VERKADA_TOKEN`)

Print effective config:

```bash
./bin/verkcli config view
```

## Footage streaming / download

The Verkada Streaming API returns HLS playlists (`.m3u8`). This CLI can:

- print a ready-to-use `.m3u8` URL
- download a historical clip as MP4 using `ffmpeg` (if installed)

You must provide your `org_id` (set it once via `--org-id` / `VERKCLI_ORG_ID` / `VERKADA_ORG_ID` or store it in your profile config).
The CLI will try to auto-discover `org_id` during `login`, but some API keys do not have permission to call the needed Core endpoint.

See also: [docs/footage.md](docs/footage.md) for more examples and troubleshooting.

Examples:

```bash
# Print an HLS URL (historical)
./bin/verkcli --org-id ORG123 cameras footage url --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z

# Download an MP4 clip (requires ffmpeg)
./bin/verkcli --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z \
  --out clip.mp4
```

## Agent skill (Codex)

This repo includes a Codex skill at `.agents/skills/verkcli` that teaches agents how to use the `verkcli` CLI.

Install it locally by symlinking into `~/.codex/skills` (run from the repo root):

```bash
mkdir -p ~/.codex/skills
ln -sfn "$PWD/.agents/skills/verkcli" ~/.codex/skills/verkcli
```
