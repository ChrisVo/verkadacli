# Thumbnail Timestamp Handling

The `cameras thumbnail` command accepts the same `--timestamp` input patterns as before:

- Unix seconds (e.g. `1736893300`)
- RFC3339 with offset (e.g. `2026-02-15T14:30:00Z` or `2026-02-15T07:30:00-07:00`)
- RFC3339 without timezone (e.g. `2026-02-15T14:30:00`)
- Local datetime (e.g. `2026-02-15 14:30:00`)

Naive forms (without timezone) are parsed using `--tz`.

## `--tz`

`--tz` is only used when `--timestamp` is naive (no offset).
- Default: `local`
- Common values: `America/Los_Angeles`, `America/New_York`, `UTC`
- Invalid values return an error: `invalid --tz value ...`

Examples:

```bash
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp 2026-02-13T08:00:00 --tz America/Los_Angeles
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp "2026-02-13 08:00:00" --tz America/Los_Angeles
./bin/verkcli cameras thumbnail --camera-id CAM123 --timestamp 2026-02-13T16:00:00Z
```

## Important endpoint behavior

The API returns the **closest cached thumbnail** to the requested time, not always an exact frame.
Expected behavior:

- Normal operation: roughly every 60 seconds.
- If a person is detected: up to 6 additional thumbnails in that minute.
- No motion: refreshes can be several minutes apart.

If you request a specific moment, minor skew (often up to minutes) is expected by design.
