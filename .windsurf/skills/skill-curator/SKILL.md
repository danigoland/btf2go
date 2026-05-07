---
name: skill-curator
description: curate a small, high-value project skill set from a larger catalog. use when chatgpt should analyze a repository, dependency files, config, readme, optional plan or spec markdown, and catalog.md, then select 10-15 relevant skills, always include baseline review skills, validate skill names against the catalog, and update a local .agents folder plus agent-specific project skill folders by calling the bundled helper scripts.
---

# Skill Curator

## Overview

Use this skill to keep a project-specific subset of skills aligned with the current codebase and plans. The language model does the reasoning and selection. Bundled scripts only perform deterministic filesystem, validation, and manifest tasks.

## Required inputs

Gather as many of these as are available before selecting skills:

1. Project root path.
2. Optional plan/spec markdown files.
3. Global catalog path. Default: `~/.agent-sources/antigravity-awesome-skills`.
4. Existing `.agents/skills.lock.json` if present.
5. Existing `.agents/curator-policy.yaml` if present.
6. Preferred copy mode: `copy` or `symlink`.

## Workflow

Follow this sequence every time.

### 1) Inspect the project

Read the repo structure and available docs. Prioritize:

- `README*`
- `package.json`, `pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`
- `pyproject.toml`, `requirements*.txt`, `poetry.lock`
- `Cargo.toml`, `go.mod`, `Gemfile`, `pom.xml`, `build.gradle*`
- `Dockerfile*`, `docker-compose*`, `.github/workflows/*`
- framework config files
- `docs/`, `specs/`, `plans/`, ADRs, RFCs
- any user-provided spec/plan files

Infer:

- primary and secondary languages
- frameworks
- major libraries and infra
- repo type: backend, frontend, fullstack, mobile, infra, notebook-heavy, agentic
- domain: scientific, finance, quant, ai-agent, saas, etc.
- project phase: prototyping, delivery, stabilization, production hardening, scaling
- whether UI is present

### 2) Read the policy and prior state

Read these files if present:

- `references/selection-policy.yaml`
- `references/domain-signals.md`
- `.agents/curator-policy.yaml`
- `.agents/skills.lock.json`

Use the project-local policy file to override or refine defaults when it exists.

### 3) Read the skill catalog

Read `CATALOG.md` from the configured catalog path. Use it as the source of truth for candidate skill names and short descriptions. Prefer exact catalog names when passing the final selection to scripts.

If the catalog is large, do not brute-force every skill folder first. Start with `CATALOG.md`, shortlist candidates, then inspect only the most relevant skill folders when you need disambiguation.

### 4) Apply the curation policy

Always try to include these categories:

- security review
- quality or correctness review
- performance review
- testing
- architecture or refactor
- debugging or root-cause analysis

Include a UI skill when the project has clear UI signals.

Then fill remaining slots using this priority order:

1. language
2. framework
3. major libraries and infra
4. domain
5. current phase

Prefer one strong skill per concern over multiple overlapping skills.
Prefer framework-specific skills over generic skills.
Prefer skills that will matter in the next 2-4 weeks.

### 5) Produce the final selection

Select 10-15 skills total. Target this shape unless the repo strongly suggests otherwise:

- 6 baseline review/debug/architecture/testing skills
- 1 UI skill when applicable
- 3-5 stack skills
- 2-4 domain or infra skills

For each selected skill, keep:

- `name`
- `category`
- `reason`

### 6) Apply the selection

After deciding on the final skill names, call the helper script:

```bash
python3 scripts/apply_selection.py \
  --project-root <project-root> \
  --catalog-root <catalog-root> \
  --copy-mode <copy|symlink> \
  --selection-json '[
    {"name":"security-review","category":"baseline","reason":"baseline required"},
    {"name":"performance-review","category":"baseline","reason":"baseline required"}
  ]'
```

Also pass repeated `--plan-file` flags when plan/spec docs were used.

Pass repeated `--sync-agent` flags when you want to refresh specific agent-local project folders. Default sync targets are:

- antigravity
- claude-code
- codex
- gemini-cli
- windsurf
- droid

The script will only perform the deterministic apply phase:

- validate selected skill names against `CATALOG.md`
- create `.agents/` if needed
- create `.agents/selected-skills/`
- create `.agents/curator-policy.yaml` if missing
- copy or symlink the selected skill directories
- write `.agents/skills.lock.json`
- write `.agents/SKILLS.md`
- write `.agents/rerun-curator.sh`
- sync the curated subset into agent-specific project folders

Do not treat the script as the selection engine. The model must decide the final skill list before calling it.

### 7) Final response to the user

Report:

- detected languages, frameworks, libraries, domain, and phase
- final selected skills grouped by category
- why each was chosen
- whether `.agents/` was created or refreshed
- whether copy or symlink mode was used
- which agent-local folders were synced

## Quality bar

Before applying the selection, check all of the following:

- baseline review categories are covered
- UI was included when UI is present
- no obvious duplicate or overlapping picks remain
- selected skills map to real catalog entries
- the total stays within 10-15 unless the user explicitly asks otherwise
- the final set is practical, not exhaustive

## Bundled references

- `references/selection-policy.yaml`: editable default policy and slot allocation
- `references/domain-signals.md`: domain detection hints
- `references/folder-conventions.md`: output paths and sync behavior
- `references/output-examples.md`: example lockfile and summary output

## Bundled scripts

- `scripts/bootstrap_catalog.sh`: clone or pull the catalog repo into the global path
- `scripts/parse_catalog.py`: parse `CATALOG.md` into machine-readable JSON
- `scripts/apply_selection.py`: validate names, create `.agents/`, copy/symlink selected skills, write manifests, sync agent-local folders
- `scripts/run_curator.sh`: deterministic local helper that documents the apply phase when invoked inside the selected skill folder
- `scripts/install_global.sh`: install this skill globally for common coding agents via `npx skills add`
