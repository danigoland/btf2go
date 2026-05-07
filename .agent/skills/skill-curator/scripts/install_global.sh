#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILL_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

AGENTS=(antigravity claude-code codex gemini-cli windsurf droid)

while [ "$#" -gt 0 ]; do
  case "$1" in
    --agent)
      AGENTS+=("$2")
      shift 2
      ;;
    -h|--help)
      echo "Usage: install_global.sh [--agent NAME]..."
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

cmd=(npx skills add "$SKILL_ROOT" -g -y)
for agent in "${AGENTS[@]}"; do
  cmd+=(-a "$agent")
done
"${cmd[@]}"
