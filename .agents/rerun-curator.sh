#!/usr/bin/env bash
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG_ROOT="/Users/dani/.agent-sources/antigravity-awesome-skills"
COPY_MODE="copy"
SCRIPT_DIR="$PROJECT_ROOT/.agents/selected-skills/skill-curator/scripts"
if [ ! -d "$SCRIPT_DIR" ]; then
  echo "Could not locate skill-curator scripts under .agents/selected-skills" >&2
  exit 1
fi
CMD=("$SCRIPT_DIR/run_curator.sh" "$PROJECT_ROOT" "--catalog-root" "$CATALOG_ROOT" "--copy-mode" "$COPY_MODE")
CMD+=("--sync-agent" "antigravity")
CMD+=("--sync-agent" "claude-code")
CMD+=("--sync-agent" "codex")
CMD+=("--sync-agent" "gemini-cli")
CMD+=("--sync-agent" "windsurf")
CMD+=("--sync-agent" "droid")
CMD+=("--plan-file" "/Users/dani/btf2go/docs/superpowers/specs/2026-05-06-btf2go-v0.1-design.md")
CMD+=("--plan-file" "/Users/dani/btf2go/docs/superpowers/plans/2026-05-06-btf2go-v0.1.md")
CMD+=("--plan-file" "/Users/dani/btf2go/CHANGELOG.md")
"${CMD[@]}"
