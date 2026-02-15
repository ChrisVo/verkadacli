# verkadacli

Go CLI to interface with Verkada.

## Status

This is a starter CLI skeleton using Cobra. It includes:

- `verkada config init` to create a local config file
- `verkada config view` to print effective config
- `verkada request` to make raw HTTP requests (until typed commands are added)

## Build

This repo assumes you have Go installed.

```sh
mkdir -p bin
go build -o bin/verkada ./cmd/verkada
```

If you're in a restricted environment where Go can't write to the default build cache (e.g. `~/Library/Caches/go-build`), use:

```sh
make build
make test
```

## Login

Save your base URL and API key to the local config file:

```sh
./bin/verkada login --base-url https://api.verkada.com --api-key "$VERKADA_API_KEY"
```

Or run `./bin/verkada login` to be prompted and save to config.

## Auth / Config

Config defaults to `$XDG_CONFIG_HOME/verkada/config.json` (often `~/.config/verkada/config.json`).

Supported env vars:

- `VERKADA_BASE_URL`
- `VERKADA_API_KEY`
- `VERKADA_TOKEN`

You can also pass headers directly:

```sh
./bin/verkada request -H 'x-api-key: ...' --method GET --path /v1/cameras
```
