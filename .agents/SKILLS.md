# Selected skills

## Detected project traits

## Curation metadata
- **Policy**: `/Users/dani/btf2go/.agents/curator-policy.yaml`
- **Catalog**: `/Users/dani/.agent-sources/antigravity-awesome-skills/CATALOG.md`

## Baseline
- **architect-review** — baseline architecture review for the 5-phase pipeline
- **code-reviewer** — baseline quality review on each PR
- **systematic-debugging** — baseline; bit-math + alignment edge cases are highest-risk
- **verification-before-completion** — baseline; layout verifier + golden tests must actually pass before merge
- **test-driven-development** — baseline testing discipline (replaces tdd-workflow which overlapped)
- **cc-skill-security-review** — baseline; ELF parser must not panic on hostile BTF input

## Domain
- **binary-analysis-patterns** — core domain: parsing BTF from compiled ELF artifacts
- **memory-safety-patterns** — core domain: alignment/padding/packed-struct downgrade math

## Infra
- **github-actions-templates** — NEW v0.1.1: CI matrix with pinned SHAs and govulncheck just landed; future workflow tweaks benefit from this skill

## Quality
- **error-handling-patterns** — BTF loader, resolver, codegen, and CLI must surface clear errors at boundaries

## Stack
- **golang-pro** — primary language; idiomatic Go 1.22+ patterns
- **rust-pro** — NEW v0.1.1: Aya kernel fixture in tests/fixtures/rust uses Rust; future maintenance touches Cargo.toml + bpf-linker setup

## Agent sync
- **antigravity** → `/Users/dani/btf2go/.agent/skills` (15 skills)
- **claude-code** → `/Users/dani/btf2go/.claude/skills` (15 skills)
- **codex** → `/Users/dani/btf2go/.agents/skills` (15 skills)
- **gemini-cli** → `/Users/dani/btf2go/.agents/skills` (15 skills)
- **windsurf** → `/Users/dani/btf2go/.windsurf/skills` (15 skills)
- **droid** → `/Users/dani/btf2go/.factory/skills` (15 skills)
