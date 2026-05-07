# Output examples

## Example `.agents/skills.lock.json`

```json
{
  "selection_policy_version": 5,
  "catalog_root": "~/.agent-sources/antigravity-awesome-skills",
  "copy_mode": "copy",
  "detected": {
    "languages": ["python", "typescript"],
    "frameworks": ["nextjs", "fastapi"],
    "domain": ["ai-agent"],
    "ui_present": true
  },
  "selected_skills": [
    {
      "name": "security-review",
      "category": "baseline",
      "reason": "baseline required"
    }
  ]
}
```

## Example `.agents/SKILLS.md`

```md
# Selected skills

## Detected project traits
- **languages**: ['python', 'typescript']
- **frameworks**: ['nextjs', 'fastapi']
- **domain**: ['ai-agent']
- **ui_present**: True

## Baseline
- **security-review** — baseline required
- **performance-review** — baseline required

## Stack
- **fastapi-expert** — core backend framework detected
- **nextjs-review** — ui and framework detected
```
