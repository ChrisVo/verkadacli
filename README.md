## 📷 verkadacli — Inspect cameras, fetch thumbnails, and script Verkada APIs

`verkada` is a Go CLI for Verkada APIs. It supports named profiles (multiple orgs/regions), typed camera commands, local camera labels, and a raw `request` escape hatch.

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
- **Raw HTTP**: `verkada request ...` for endpoints we haven’t typed yet.

## Requirements

- Go (for building from source).
- Network access to the Verkada API.

Important:
- Your API base URL should be an API host like `https://api.verkada.com` (or `https://api.eu.verkada.com`, `https://api.au.verkada.com`).
- Do not use `*.command.verkada.com` (that is the web UI; you’ll get HTML).

## Install / build

## Homebrew

If you have Homebrew installed:

```bash
brew install ChrisVo/tap/verkadacli
verkada --help
```

(Formula name is `verkadacli`, installed binary is `verkada`.)

Build a local binary:

```bash
mkdir -p bin
go build -o bin/verkada ./cmd/verkada
./bin/verkada --help
```

If you’re in an environment where Go can’t write to the default build cache, use:

```bash
make build
make test
```

## Quick start

Create a config:

```bash
./bin/verkada config init
```

Login (saves API key and base URL under a profile):

```bash
./bin/verkada login
```

Or non-interactive:

```bash
./bin/verkada --profile default login --no-prompt \
  --base-url https://api.verkada.com \
  --org-id "YOUR_ORG_ID" \
  --api-key "$VERKADA_API_KEY"
```

By default, `login` runs a preflight check (it verifies the API key can list cameras and that the streaming `.m3u8` endpoint works for your `org_id`).
Skip this if you only want to write config:

```bash
./bin/verkada login --no-verify ...
```

List cameras (text table):

```bash
./bin/verkada cameras list
./bin/verkada cameras list --wide
./bin/verkada cameras list --all
./bin/verkada cameras list --all --q lobby
./bin/verkada cameras list --camera-id <camera_id>
```

Get one camera:

```bash
./bin/verkada cameras get <camera_id>
./bin/verkada --output json cameras get <camera_id>
```

Fetch a thumbnail:

```bash
./bin/verkada cameras thumbnail --camera-id <camera_id>
./bin/verkada cameras thumbnail --camera-id <camera_id> --resolution hi-res --out thumb.jpg
./bin/verkada cameras thumbnail --camera-id <camera_id> --timestamp 1736893300 --out thumb.jpg
./bin/verkada cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-15T14:30:00Z --out thumb.jpg
./bin/verkada cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13T08:00:00 --tz America/Los_Angeles --out thumb.jpg
./bin/verkada cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13 08:00:00 --tz America/Los_Angeles
./bin/verkada cameras thumbnail --camera-id <camera_id> --timestamp 2026-02-13T08:00:00Z --tz America/Los_Angeles
  -> `--tz` is ignored when --timestamp includes an explicit offset like `Z` or `-07:00`.

# Inline render in iTerm2/WezTerm (use --out if you also want a file)
./bin/verkada cameras thumbnail --camera-id <camera_id> --view
./bin/verkada cameras thumbnail --camera-id <camera_id> --view --out thumb.jpg
```

See also: [docs/thumbnail.md](docs/thumbnail.md) for full timestamp and timezone rules.

## Profiles

Pick a profile per command:

```bash
./bin/verkada --profile eu cameras list
```

Set a default profile:

```bash
./bin/verkada config profiles list
./bin/verkada config use eu
```

## Local camera labels

Labels are stored locally in your config profile and show up in `cameras list` output.

```bash
./bin/verkada cameras label set <camera_id> "Front Door"
./bin/verkada cameras label list
./bin/verkada cameras label rm <camera_id>
```

Filter using labels:

```bash
./bin/verkada cameras list --all --q "front"
```

## Raw requests

Use typed commands when available; otherwise:

```bash
./bin/verkada request --method GET --path /cameras/v1/devices
./bin/verkada request -H "x-api-key: $VERKADA_API_KEY" --method POST --path /token
```

## Config

Config defaults to `$XDG_CONFIG_HOME/verkada/config.json` (often `~/.config/verkada/config.json`).

Supported env vars:

- `VERKADA_PROFILE`
- `VERKADA_BASE_URL`
- `VERKADA_ORG_ID`
- `VERKADA_API_KEY`
- `VERKADA_TOKEN`

Print effective config:

```bash
./bin/verkada config view
```

## Footage streaming / download

The Verkada Streaming API returns HLS playlists (`.m3u8`). This CLI can:

- print a ready-to-use `.m3u8` URL
- download a historical clip as MP4 using `ffmpeg` (if installed)

You must provide your `org_id` (set it once via `--org-id` / `VERKADA_ORG_ID` or store it in your profile config).
The CLI will try to auto-discover `org_id` during `login`, but some API keys do not have permission to call the needed Core endpoint.

Examples:

```bash
# Print an HLS URL (historical)
./bin/verkada --org-id ORG123 cameras footage url --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z

# Download an MP4 clip (requires ffmpeg)
./bin/verkada --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z \
  --out clip.mp4
```
