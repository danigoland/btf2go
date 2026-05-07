Use the global skill-curator skill on this repository.

Project root: /Users/dani/btf2go
Catalog root: /Users/dani/.agent-sources/antigravity-awesome-skills
Copy mode: copy

Follow the skill instructions to:
1. Analyze the codebase and any plan/spec docs.
2. Read CATALOG.md from the catalog root.
3. Select 10-15 skills, always including the baseline review categories.
4. Minimize overlap and prefer framework-specific skills where appropriate.
5. Call the bundled apply_selection.py helper to write .agents/, selected-skills, skills.lock.json, and SKILLS.md.

Use these sync targets unless the repo or task suggests otherwise:
- antigravity
- claude-code
- codex
- gemini-cli
- windsurf
- droid

Use this helper command after you decide on the final selection:

```bash
python3 .agents/selected-skills/skill-curator/scripts/apply_selection.py \
  --project-root /Users/dani/btf2go \
  --catalog-root /Users/dani/.agent-sources/antigravity-awesome-skills \
  --copy-mode copy \
  --sync-agent antigravity \
  --sync-agent claude-code \
  --sync-agent codex \
  --sync-agent gemini-cli \
  --sync-agent windsurf \
  --sync-agent droid \
  --selection-json '[{name:security-review,category:baseline,reason:baseline required}]'
```
