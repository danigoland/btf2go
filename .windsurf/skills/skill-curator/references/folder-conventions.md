# Folder conventions

## Project-local curated output

The curator writes the canonical project subset to:

- `.agents/selected-skills/`
- `.agents/skills.lock.json`
- `.agents/SKILLS.md`
- `.agents/curator-policy.yaml`
- `.agents/rerun-curator.sh`

Use `.agents/selected-skills/` as the neutral source of truth for the project.

## Agent-specific project sync targets

These paths match current `skills` CLI agent locations for project installs when available:

- Antigravity → `.agent/skills/`
- Claude Code → `.claude/skills/`
- Codex → `.agents/skills/`
- Gemini CLI → `.agents/skills/`
- Windsurf → `.windsurf/skills/`
- Droid → `.factory/skills/`

The apply script can copy or symlink the curated project subset into these folders.

## Notes

- `.agents/selected-skills/` is the canonical curated subset.
- Agent-specific folders are synced views for tool compatibility.
- Re-running the curator should refresh both the canonical subset and the synced agent views.
