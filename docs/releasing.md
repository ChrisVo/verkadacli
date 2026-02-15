# Releasing (GitHub + Homebrew)

This repo is set up to publish GitHub Releases with GoReleaser and to update a Homebrew tap so users can run:

```bash
brew install chrisvo/tap/verkcli
```

## One-time setup

1. Ensure the CLI repo is public and lives at `ChrisVo/verkcli` on GitHub (the GoReleaser config pins releases to that repo).
2. Create the Homebrew tap repo on GitHub:
   - Repo name must be `ChrisVo/homebrew-tap` (Homebrew maps `chrisvo/tap` to `chrisvo/homebrew-tap`).
   - It must be public for unauthenticated `brew install` to work.
3. Create a GitHub classic PAT that can push to `ChrisVo/homebrew-tap` and add it to the `verkcli` repo as an Actions secret:
   - Secret name: `HOMEBREW_TAP_GITHUB_TOKEN`

## Cutting a release

1. Create and push a version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

2. GitHub Actions runs `.github/workflows/release.yml`, which:
   - builds cross-platform tarballs
   - creates a GitHub Release
   - updates `ChrisVo/homebrew-tap` with `Formula/verkcli.rb`

## Verifying Homebrew install

From a clean machine (or after `brew untap chrisvo/tap`):

```bash
brew install chrisvo/tap/verkcli
verkcli version
```

