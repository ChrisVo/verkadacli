# verkcli Repo Docs (Pointers)

Use these files as the canonical, up-to-date reference when working in the `verkcli` repo:

- `README.md`: end-to-end usage, profiles, login, cameras, thumbnails, raw request, config/env vars, footage examples.
- `docs/thumbnail.md`: timestamp parsing rules and how `--tz` is applied; notes about cached thumbnail behavior.
- `docs/footage.md`: `cameras footage` usage, `org_id` requirements, HLS/ffmpeg download notes, common errors.

When you need the authoritative command surface area, run:

```bash
./bin/verkcli --help
./bin/verkcli cameras --help
./bin/verkcli cameras thumbnail --help
./bin/verkcli cameras footage --help
./bin/verkcli request --help
./bin/verkcli config --help
```
