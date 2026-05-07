#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CATALOG_ROOT="${ANTIGRAVITY_SKILLS_DIR:-$HOME/.agent-sources/antigravity-awesome-skills}"
COPY_MODE="copy"
PLAN_FILES=()
SYNC_AGENTS=(antigravity claude-code codex gemini-cli windsurf droid)

usage() {
  cat <<EOF
Usage: run_curator.sh <project-root> [options]

This script is the deterministic apply phase. The LLM should inspect the repo,
read CATALOG.md, select 10-15 skills, then call apply_selection.py.

Options:
  --catalog-root PATH     Override the global catalog root
  --copy-mode MODE        copy or symlink (default: copy)
  --plan-file PATH        Attach a plan/spec markdown file (repeatable)
  --sync-agent NAME       Sync curated skills into a project-local agent folder (repeatable)
  -h, --help              Show help
EOF
}

if [ "$#" -lt 1 ]; then
  usage >&2
  exit 1
fi

PROJECT_ROOT="$1"
shift

while [ "$#" -gt 0 ]; do
  case "$1" in
    --catalog-root)
      CATALOG_ROOT="$2"
      shift 2
      ;;
    --copy-mode)
      COPY_MODE="$2"
      shift 2
      ;;
    --plan-file)
      PLAN_FILES+=("$2")
      shift 2
      ;;
    --sync-agent)
      SYNC_AGENTS+=("$2")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ ! -f "$CATALOG_ROOT/CATALOG.md" ] && [ ! -f "$CATALOG_ROOT/catalog.md" ]; then
  echo "Missing CATALOG.md under $CATALOG_ROOT" >&2
  exit 1
fi

cat <<EOF
Repository analysis and skill selection should be done by the LLM using SKILL.md.
Then invoke apply_selection.py like this:

python3 "$SCRIPT_DIR/apply_selection.py" \
  --project-root "$PROJECT_ROOT" \
  --catalog-root "$CATALOG_ROOT" \
  --copy-mode "$COPY_MODE" \
  --selection-json '[{"name":"security-review","category":"baseline","reason":"baseline required"}]'
EOF

if [ "${APPLY_SELECTION_NOW:-0}" = "1" ]; then
  cmd=(python3 "$SCRIPT_DIR/apply_selection.py" --project-root "$PROJECT_ROOT" --catalog-root "$CATALOG_ROOT" --copy-mode "$COPY_MODE")
  for pf in "${PLAN_FILES[@]}"; do
    cmd+=(--plan-file "$pf")
  done
  for agent in "${SYNC_AGENTS[@]}"; do
    cmd+=(--sync-agent "$agent")
  done
  if [ -n "${SELECTION_JSON:-}" ]; then
    cmd+=(--selection-json "$SELECTION_JSON")
  else
    echo "APPLY_SELECTION_NOW=1 requires SELECTION_JSON" >&2
    exit 1
  fi
  "${cmd[@]}"
fi
