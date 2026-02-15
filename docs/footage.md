# Footage Streaming / Download

The `cameras footage` commands use Verkada's Streaming API (HLS playlists, `.m3u8`).

## Org ID (`org_id`) is required

Streaming endpoints require an `org_id`. You can provide it via:

- `--org-id ORG123` (recommended for one-off commands)
- `VERKCLI_ORG_ID=ORG123` (recommended for shells/CI; legacy: `VERKADA_ORG_ID`)
- storing it in your profile config (written by `verkcli login` when available)

During `verkcli login`, the CLI will try to auto-discover your `org_id` via a Core API call. Some API keys do not have permission for this; in that case you must set it manually.

If your API key has access, you can often retrieve it with:

```bash
./bin/verkcli --output json request --method GET --path /core/v1/organization
```

Look for an `org_id` (or similar) field in the JSON response.

## Print an HLS URL

Historical window:

```bash
./bin/verkcli --org-id ORG123 cameras footage url --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z
```

Live:

```bash
./bin/verkcli --org-id ORG123 cameras footage url --camera-id CAM123 --live
```

If you omit both `--start` and `--end`, the command defaults to live.

## Download an MP4 clip (requires `ffmpeg`)

```bash
./bin/verkcli --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z \
  --out clip.mp4
```

If you want to see exactly what will run:

```bash
./bin/verkcli --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z \
  --out clip.mp4 --print-ffmpeg
```

## Time Formats and `--tz`

`--start`/`--end` accept:

- Unix seconds
- RFC3339 with offset (e.g. `2026-02-15T14:00:00Z`, `2026-02-15T07:00:00-07:00`)
- RFC3339 without timezone (e.g. `2026-02-15T14:00:00`)
- `YYYY-MM-DD HH:MM:SS`

Naive forms (no offset) are interpreted using `--tz` (default: `local`):

```bash
./bin/verkcli --org-id ORG123 cameras footage download --camera-id CAM123 \
  --start "2026-02-15 06:00:00" --end "2026-02-15 06:05:00" --tz America/Los_Angeles \
  --out clip.mp4
```

## Common Errors

- `org id is empty ...`: set `--org-id` / `VERKCLI_ORG_ID` / `VERKADA_ORG_ID`, or re-run `verkcli login` with `--org-id`.
- `received HTML instead of m3u8`: your `--base-url` is likely wrong (must be `https://api(.eu|.au).verkada.com`) or your org/camera permissions don't match.
- `ffmpeg not found in PATH`: install `ffmpeg`, or use `cameras footage url` and download with your own HLS tooling.
