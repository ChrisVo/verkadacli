---
name: verkcli
description: "Use when asked to use the `verkcli` CLI to interact with Verkada APIs: initialize/configure profiles, login and manage API keys/tokens, list/get cameras, fetch thumbnails, stream/download footage, or make raw HTTP requests via `verkcli request`. Also use to troubleshoot common issues (wrong base URL, missing org_id, HTML responses, ffmpeg missing, auth/token headers)."
---

# verkcli

## Quick start

Prefer typed commands (`cameras ...`) over `request` when possible. When unsure about flags, run `verkcli --help` or `verkcli <cmd> --help`.

From this repo:

```bash
mkdir -p bin
go build -o bin/verkcli ./cmd/verkcli
./bin/verkcli --help
```

Initialize config and login:

```bash
./bin/verkcli config init
./bin/verkcli login
```

Non-interactive login (good for CI):

```bash
./bin/verkcli --profile default login --no-prompt \
  --base-url https://api.verkada.com \
  --org-id "YOUR_ORG_ID" \
  --api-key "$VERKCLI_API_KEY"
```

## Config and profiles

- Config file default: `$XDG_CONFIG_HOME/verkcli/config.json` (often `~/.config/verkcli/config.json`). Legacy configs at `$XDG_CONFIG_HOME/verkada/config.json` are still read.
- Select profile via `--profile` or `VERKCLI_PROFILE` (legacy: `VERKADA_PROFILE`).
- Print effective config (file + env + flags): `verkcli config view`.

Profile commands:

```bash
./bin/verkcli config profiles list
./bin/verkcli config use eu
./bin/verkcli --profile eu cameras list
```

## Authentication and base URL

- Use API hosts like `https://api.verkada.com` (or `.eu`, `.au`). Avoid `*.command.verkada.com` (web UI).
- Provide secrets via flags or env vars:
  - `VERKCLI_API_KEY`, `VERKCLI_TOKEN` (legacy: `VERKADA_API_KEY`, `VERKADA_TOKEN`)
  - `VERKCLI_BASE_URL`, `VERKCLI_ORG_ID` (legacy: `VERKADA_BASE_URL`, `VERKADA_ORG_ID`)
- The CLI can auto-fetch `x-verkada-auth` by `POST /token` using `x-api-key` (then retries once).

## Cameras

List and filter:

```bash
./bin/verkcli cameras list
./bin/verkcli cameras list --wide
./bin/verkcli cameras list --all --q lobby
./bin/verkcli --output json cameras list --all
```

Get one camera:

```bash
./bin/verkcli cameras get CAM123
./bin/verkcli --output json cameras get CAM123
```

Local labels (stored in profile config):

```bash
./bin/verkcli cameras label set CAM123 "Front Door"
./bin/verkcli cameras label list
./bin/verkcli cameras label rm CAM123
```

## Thumbnails

The backend can return the closest cached thumbnail (skew of minutes is normal when no motion exists).

Common patterns:

```bash
./bin/verkcli cameras thumbnail --camera-id CAM123 --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp 1736893300 --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp 2026-02-15T14:30:00Z --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp "2026-02-13 08:00:00" --tz America/Los_Angeles --out thumb.jpg
./bin/verkcli cameras thumbnail --camera-id CAM123 --view
```

For the full timestamp and timezone rules, read `docs/thumbnail.md` in this repo.

## Footage (stream/download)

Footage endpoints require `org_id` (`--org-id` or `VERKCLI_ORG_ID` / `VERKADA_ORG_ID`).

```bash
./bin/verkcli --org-id ORG123 cameras footage url --camera-id CAM123 --live
./bin/verkcli --org-id ORG123 cameras footage url --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z
./bin/verkcli --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z --out clip.mp4
```

Footage docs and troubleshooting live in `docs/footage.md` in this repo.

## Raw HTTP requests

Use when a typed command does not exist yet.

```bash
./bin/verkcli request --method GET --path /cameras/v1/devices
./bin/verkcli request --show-headers --method GET --path /core/v1/organization
./bin/verkcli request --method POST --path /token -H "x-api-key: $VERKCLI_API_KEY"
```

## Troubleshooting

- HTML responses: base URL is likely wrong (must be `https://api(.eu|.au).verkada.com`, not the web UI host).
- `org id is empty`: provide `--org-id` / `VERKCLI_ORG_ID` / `VERKADA_ORG_ID` or re-run `verkcli login` with `--org-id`.
- `ffmpeg not found`: install `ffmpeg`, or use `cameras footage url` and download with your own tooling.

## References

If more detail is needed, read `references/repo-docs.md`.
