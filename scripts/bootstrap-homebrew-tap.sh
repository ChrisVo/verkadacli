#!/usr/bin/env bash
set -euo pipefail

# Bootstrap the GitHub tap repo that Homebrew expects for `brew install chrisvo/tap/verkcli`.
# This script is intended for maintainers; it does not run in CI.

OWNER="${OWNER:-ChrisVo}"
REPO="${REPO:-homebrew-tap}"

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh (GitHub CLI) is required" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh is not authenticated; run: gh auth login" >&2
  exit 1
fi

if gh repo view "${OWNER}/${REPO}" >/dev/null 2>&1; then
  echo "tap repo already exists: ${OWNER}/${REPO}" >&2
  exit 0
fi

echo "creating tap repo: ${OWNER}/${REPO}" >&2
gh repo create "${OWNER}/${REPO}" --public --description "Homebrew tap for verkcli" --confirm

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

git clone "https://github.com/${OWNER}/${REPO}.git" "$tmp/${REPO}" >/dev/null 2>&1
cd "$tmp/${REPO}"

mkdir -p Formula
cat >README.md <<'EOF'
# Homebrew tap

This repository is a Homebrew tap.
Install the formula with:

```bash
brew install chrisvo/tap/verkcli
```

The formula is maintained by GoReleaser from the `verkcli` repo.
EOF

git add README.md Formula
git commit -m "Initial tap repo" >/dev/null 2>&1 || true
git branch -M main
git push -u origin main >/dev/null 2>&1

echo "created: https://github.com/${OWNER}/${REPO}" >&2

