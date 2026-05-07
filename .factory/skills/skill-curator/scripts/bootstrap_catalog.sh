#!/usr/bin/env bash
set -euo pipefail
CATALOG_DIR="${1:-$HOME/.agent-sources/antigravity-awesome-skills}"
REPO_URL="${2:-https://github.com/sickn33/antigravity-awesome-skills.git}"
mkdir -p "$(dirname "$CATALOG_DIR")"
if [ -d "$CATALOG_DIR/.git" ]; then
  git -C "$CATALOG_DIR" pull --ff-only
else
  git clone "$REPO_URL" "$CATALOG_DIR"
fi
