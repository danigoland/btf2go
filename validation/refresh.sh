#!/usr/bin/env bash
# refresh.sh — materialize the validation corpus from manifest.yaml.
#
# Reads the YAML manifest, clones each entry into validation/corpus/
# at the pinned commit, and runs the build command. Idempotent: if
# a project is already cloned, fetches and resets to the pin.

set -euo pipefail

MANIFEST="${MANIFEST:-$(dirname "$0")/corpus/manifest.yaml}"
CORPUS_DIR="$(dirname "$0")/corpus"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "refresh.sh: missing required tool: $1" >&2
    exit 1
  }
}

require yq
require git
require make

echo "[refresh] manifest: $MANIFEST"

# C corpus
yq -r '.c_corpus[] | [.name, .source_url, .pinned_commit, .build.cmd] | @tsv' "$MANIFEST" |
while IFS=$'\t' read -r name url commit cmd; do
  dest="$CORPUS_DIR/c/$name"
  echo "[refresh] C: $name @ $commit"
  if [ -d "$dest/.git" ]; then
    git -C "$dest" fetch --quiet origin
  else
    mkdir -p "$(dirname "$dest")"
    git clone --quiet "$url" "$dest"
  fi
  git -C "$dest" -c advice.detachedHead=false checkout --quiet "$commit"
  if [ -n "$cmd" ]; then
    echo "[refresh]   build (errors are non-fatal — toolchain may be missing): $cmd"
    (cd "$dest" && eval "$cmd") || echo "[refresh]   build failed for $name (continuing)"
  fi
done

# Aya corpus
yq -r '.aya_corpus[] | [.name, .source_url, .pinned_commit, .build.cmd] | @tsv' "$MANIFEST" |
while IFS=$'\t' read -r name url commit cmd; do
  dest="$CORPUS_DIR/aya/$name"
  echo "[refresh] Aya: $name @ $commit"
  if [ -d "$dest/.git" ]; then
    git -C "$dest" fetch --quiet origin
  else
    mkdir -p "$(dirname "$dest")"
    git clone --quiet "$url" "$dest"
  fi
  git -C "$dest" -c advice.detachedHead=false checkout --quiet "$commit"
  if [ -n "$cmd" ]; then
    echo "[refresh]   build (errors are non-fatal — toolchain may be missing): $cmd"
    (cd "$dest" && eval "$cmd") || echo "[refresh]   build failed for $name (continuing)"
  fi
done

echo "[refresh] done"
