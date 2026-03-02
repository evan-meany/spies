#!/usr/bin/env bash
set -euo pipefail

: "${BUMP_TYPE:?BUMP_TYPE env var required (major|minor|patch)}"
: "${PR_BODY:?PR_BODY env var required}"

# --- VERSION bump ---
if [[ ! -f VERSION ]]; then
  echo "VERSION file not found at repo root."
  exit 1
fi

CURRENT_VERSION="$(tr -d ' \n\r\t' < VERSION)"

if ! [[ "$CURRENT_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "VERSION must be in X.Y.Z format. Found: '$CURRENT_VERSION'"
  exit 1
fi

IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

case "$BUMP_TYPE" in
  major) ((MAJOR += 1)); MINOR=0; PATCH=0 ;;
  minor) ((MINOR += 1)); PATCH=0 ;;
  patch) ((PATCH += 1)) ;;
  *) echo "Unknown bump type: $BUMP_TYPE"; exit 1 ;;
esac

NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"
echo "$NEW_VERSION" > VERSION

# --- CHANGELOG update ---
if [[ ! -f CHANGELOG.md ]]; then
  printf "# Changelog\n\n" > CHANGELOG.md
fi

CLEAN_ENTRY="$(python3 scripts/extract_changelog.py)"

if [[ -z "${CLEAN_ENTRY//[[:space:]]/}" ]]; then
  echo "❌ No changelog content found in PR description under ### Added/Changed/Removed."
  exit 1
fi

tmp="$(mktemp)"
{
  echo "## v$NEW_VERSION"
  echo "$CLEAN_ENTRY"
  echo
  cat CHANGELOG.md
} > "$tmp"
mv "$tmp" CHANGELOG.md

# --- commit, tag, push ---
git config user.name "github-actions"
git config user.email "github-actions@github.com"

git add VERSION CHANGELOG.md
git commit -m "chore: bump version to v$NEW_VERSION [skip ci]"
git tag "v$NEW_VERSION"

# Push the commit and tags in one go
git push origin main --follow-tags