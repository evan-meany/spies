#!/usr/bin/env bash
set -euo pipefail

: "${NEW_VERSION:?NEW_VERSION env var required}"
: "${PR_BODY:?PR_BODY env var required}"

# Ensure files exist
if [[ ! -f VERSION ]]; then
  echo "VERSION file not found at repo root."
  exit 1
fi

if [[ ! -f CHANGELOG.md ]]; then
  printf "# Changelog\n\n" > CHANGELOG.md
fi

# Write VERSION
echo "$NEW_VERSION" > VERSION

# Build changelog entry from PR body
CLEAN_ENTRY="$(python3 scripts/extract_changelog.py)"

if [[ -z "${CLEAN_ENTRY//[[:space:]]/}" ]]; then
  echo "❌ No changelog content found in PR description under ### Added/Changed/Removed."
  exit 1
fi

# Prepend changelog chunk with exactly one blank line after
tmp="$(mktemp)"
{
  echo "## v$NEW_VERSION"
  echo "$CLEAN_ENTRY"
  echo
  cat CHANGELOG.md
} > "$tmp"
mv "$tmp" CHANGELOG.md

# Commit (no push here)
git add VERSION CHANGELOG.md
git commit -m "patch: release v$NEW_VERSION [skip ci]"