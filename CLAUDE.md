# CLAUDE.md

Guidance for Claude Code sessions in this repository. Loaded at session start.

## Session-start priming

Before answering any non-trivial question about this project, scan these in order:

1. **Graph freshness check, then read.** If `graphify-out/cost.json`'s
   mtime is older than the most recent commit on `HEAD` (compare with
   `stat -f %m graphify-out/cost.json` vs `git log -1 --format=%ct`),
   run `graphify update .` first — it's AST-only, no API cost,
   ~5–10s on this codebase. Then read `graphify-out/GRAPH_REPORT.md`
   (top 50 lines) for god nodes, communities, surprising connections.
   If `graphify-out/` doesn't exist (fresh clone), run `graphify update .`
   to build it. If `graphify` itself errors, log it and proceed with
   grep/glob — don't block on the graph.
2. `CHANGELOG.md` (top of file) — what shipped most recently and current version
3. `.agents/SKILLS.md` — which curated skills are local to this project
4. `docs/superpowers/specs/` (`ls`) — canonical design intent for active work

   > Note: `docs/superpowers/{plans,specs}/` are gitignored (local-only dev workflow). Available in your local clone but not published. Skip step 4 if directory is missing on a fresh clone.

5. **claude-mem corpus check:** Run `mcp__plugin_claude-mem_mcp-search__list_corpora`. If `btf2go-current-work` is in the list (it should be), run `prime_corpus name=btf2go-current-work` once, then `query_corpus` for past in-flight context relevant to the question. The corpus is rebuilt by `/handoff` at session end.

Don't `grep` or `Glob` source files until the graph + changelog + corpus have been consulted. For library API questions, hit `context7` before reading `vendor/` or imports.

## Project status

`btf2go` is a CLI that generates Go structs from BTF embedded in compiled eBPF ELF artifacts (clang, rustc/Aya, zig). v0.3.2 shipped 2026-05-10.

- Module path: `github.com/danigoland/btf2go`
- License: Apache-2.0; repo is **public**
- Releases: https://github.com/danigoland/btf2go/releases
- ~880 LOC across 8 internal packages plus `cmd/btf2go`
- CI matrix: linux/amd64 + linux/arm64 + macos
- Three end-to-end fixtures committed (C clang, Rust/Aya, Zig) with goldens + layout verifier
- Two demo programs: `examples/c-roundtrip`, `examples/aya-roundtrip`
- **Validation framework live** at `validation/runner/` — 5 tiers (T1 differential vs bpf2go, T2 empirical layout, T2.5 kernel round-trip, T3 Aya/Rust, T4 UX walkthrough); current baseline 24 PASS / 0 FAIL / 13 SKIP. Per-run reports archived under `validation/reports/<id>.{md,json}`.
- **Datadog telemetry live** — metrics + events flow to `us3.datadoghq.com` per run; dashboard `2n5-36z-3rc/btf2go-validation` + 3 monitors. Config as IaC under `validation/datadog/`.

The differentiator vs. `cilium/ebpf`'s `bpf2go`: `bpf2go` orchestrates a `clang` build step from `.c` source. `btf2go` reads BTF straight out of any pre-built `.elf` — works for rustc/Aya and zig outputs that `bpf2go` can't ingest.

## Architecture: 5-phase pipeline

Each phase has one responsibility. Don't collapse phases.

| Phase | Package | Job |
|-------|---------|-----|
| 1 — Ingestion | `internal/btfparser` | `btf.LoadSpec(path)` → `*btf.Spec` (auto-detects ELF vs raw `.btf`) |
| 2 — Resolve + sanitize | `internal/btfparser` | Whitelist closure of `--type` ∪ `.maps` Datasec K/V ∪ recursive deps; `btf.UnderlyingType` unwraps qualifiers; `SanitizeName` PascalCases mangled names |
| 3 — Traversal | `internal/traverse` | `btf.Type` → IR. Primitives (incl. bool detection), pointers as `Pointer[T]` (declared inline in every output file), enums (signed-aware), arrays, structs, unions, `btf.Var` unwrap |
| 4 — Alignment | `internal/align` | Pure-IR pass. Inserts `_padN`, downgrades misaligned primitives to `[N]byte`, collapses bitfield runs into `_bfN [N]byte` storage with accessor metadata. **Must NOT import `cilium/ebpf/btf`** — this isolation is what lets it be unit-tested with mock IR |
| 5 — Codegen | `internal/generator` | IR → formatted Go via `strings.Builder` + `go/format.Source`. Bitfield Get/Set accessors, union accessors via `unsafe.Pointer`, sanitized headers |

The `internal/align` no-`btf` rule is sacred. Don't break it.

## Live correctness rules (v0.3.0)

These are the ones we currently get right; any change must keep them passing:

- **bit→byte:** `byteOffset = field.Offset / 8`. BTF uses bit offsets.
- **Bitfield run:** if any member has `BitfieldSize > 0`, collapse the contiguous run into one `_bfN [N]byte` field plus per-member `Get`/`Set` accessor methods. Bitfield blocks must guard against backward overlaps (`runByteOffset < cursor` is an error).
- **Packed primitive downgrade:** if BTF places a Go primitive at an offset that violates Go natural alignment, downgrade to `[size]byte` so `gc` doesn't silently re-pad.
- **Union backing:** `_data [N/A]uintK` where `K` is the max-aligned member's width and `A = K/8`. NOT `[N]byte` — that has `Alignof = 1` and SIGBUSes on ARM64/RISC-V/MIPS when the accessor casts to `*uint64`.
- **Bool detection:** `btf.Int{Size: 1, Encoding: btf.Bool OR Name: "bool"}` → Go `bool`. Catches both clang `_Bool` and Rust `bool`.
- **Signed enum:** `btf.Enum.Signed` → underlying `intN`, values rendered with sign-extension from declared width.
- **Pointer wrapper:** `Pointer[T any] uint64` declared in every generated file. No runtime dependency on `pkg/btf2go`.
- **Header injection guard:** `opts.Source` and `opts.ToolVersion` are sanitized so newlines stay inside the leading `//` comment block.
- **CLI `--pkg`:** validated against `^[a-z_][a-z0-9_]*$` AND `go/token.IsKeyword`.

If you change any of these rules, regenerate goldens (`UPDATE_GOLDEN=1 go test ./tests/...`) and confirm the layout verifier still passes (`go test ./tests/fixtures/c/verify/...`).

## Tooling guide

### Codebase questions

- **Architecture / "how does X connect to Y" / "where is the bug"** → read `graphify-out/GRAPH_REPORT.md` first; for specific questions, run `/graphify query "..."`. **Don't grep before checking the graph.** It's faster and 30-40× cheaper in tokens.
- **Specific node detail** → `/graphify explain <name>` or `/graphify path A B`
- **Recent git history** → `git log` and PRs at `https://github.com/danigoland/btf2go/pulls`
- **Past session decisions** → `claude-mem:mem-search`. For deeper themed context, build a corpus via `claude-mem:knowledge-agent`.

### External information (in order — stop at the first that works)

- **`graphify` query** if it's about *this* codebase
- **`context7`** for library API questions — version-aware docs for `cilium/ebpf`, `cobra`, etc. Use `resolve-library-id` first to get `/org/project`, then `query-docs` with a specific question. We've hit cilium/ebpf API drift multiple times (`btf.LoadSpec` vs `LoadSpecFromELF`, `Member.Offset` type, `Datasec.Vars` shape) — never guess.
- **`exa`** for general "what's out there" — real-world ecosystem questions, ecosystem validation, post-cutoff news. Variants: `web_search_exa` (default), `web_search_advanced_exa` (date/domain/category filters), `crawling_exa` (known URL), `get_code_context_exa` (search GitHub for code patterns).
- **`firecrawl_scrape`** if `exa` surfaced a URL and you need its full content
- **`firecrawl_search` / `firecrawl_crawl`** only if `exa` + `firecrawl_scrape` aren't enough. `firecrawl` is heavier; reach for it when you need full pages, JS-heavy sites, or to crawl a docs tree.

### Reasoning

- **Bit-math / alignment edge cases** → `sequentialthinking` MCP before writing. The `internal/align` and `internal/generator` packages have the highest historical bug density in this repo (3 latent plan bugs caught here pre-v0.1.0). Trace the math out loud before committing.

### Implementation discipline

- **TDD** → invoke `superpowers:test-driven-development` before writing any test/implementation pair.
- **Before claiming work is done** → invoke `superpowers:verification-before-completion`. Run the verification commands. No "should pass" — actual command output, evidence first.
- **Code review on PRs** → CodeRabbit reviews automatically when you push to a branch with an open PR. Address findings unless you genuinely disagree (some are wrong; the cross-compiler-CI ask is intentionally deferred per the design). Read findings via `gh api repos/danigoland/btf2go/pulls/<n>/reviews`.

### Skills inventory

Curated locally at `.agents/SKILLS.md`. Use via `Skill` tool. Categories:

- **Baseline:** architect-review, code-reviewer, systematic-debugging, verification-before-completion, test-driven-development, cc-skill-security-review
- **Stack:** golang-pro, rust-pro
- **Domain:** binary-analysis-patterns, memory-safety-patterns
- **Quality:** error-handling-patterns
- **Infra:** github-actions-templates

Don't invent new skills without running `skill-curator` first.

### Datadog (project-scoped MCP + live telemetry)

`.mcp.json` wires the Datadog MCP project-scoped to `us3.datadoghq.com` with env-substituted `DD_API_KEY` / `DD_APPLICATION_KEY` pulled from `.env`. Tools surface as `mcp__datadog__*`. For dashboard / metric / event work:

- Load skill first: `mcp__datadog__load_datadog_skill datadog/dashboards-and-notebooks` (or `datadog/metrics`, etc.)
- Read: `mcp__datadog__search_datadog_dashboards`, `get_datadog_dashboard`, `search_datadog_metrics`, etc.
- Write: `upsert_datadog_dashboard` (limited) or direct API via `curl` (`https://api.us3.datadoghq.com/api/v1/dashboard/<id>`)
- The runner's emit code lives in `validation/runner/datadog.go`; gated on `DATADOG_API_KEY` env. Loud-error logging includes response body (PR #55).

Reproducible config: dashboard JSON + monitor JSONs committed under `validation/datadog/`; see that dir's README for recreate/sync curl recipes.

### Disabled MCPs

`.claude/settings.local.json` denies a list for token economy. Don't try to call:

- `sentry`, `stitch`
- `plugin_playwright_playwright`, `plugin_linear_linear`
- All `claude_ai_*` SaaS connectors (Gmail, Notion, Slack, etc.)
- `perplexity-ask` (use `exa` instead)

**Datadog MCP is intentionally NOT denied** — it's project-scoped via `.mcp.json` above. If you need any of the denied ones for a specific task, tell the user and they'll re-enable explicitly.

## How to commit

Match the existing commit style (`git log --oneline -10`):
- `feat(<package>): <imperative summary>`
- `fix(<package>): <imperative summary>`
- `test(<package>): <imperative summary>`
- `docs(<package>): <imperative summary>`
- `chore: <imperative summary>`

Always include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` in the commit message body. Use HEREDOC for multi-line bodies (see existing `git log`).

## Repo layout

```
cmd/btf2go/             cobra CLI: `generate`, `inspect`
internal/types/         IR (GoFile, GoStruct, GoField, GoEnum, GoUnion, GoBitfieldBlock, ...)
internal/align/         Phase 4: padding + packed downgrade + bitfield collapse. NO btf imports.
internal/btfparser/     Phases 1-2: load, sanitize, resolve closure
internal/traverse/      Phase 3: btf.Type → IR
internal/generator/     Phase 5: IR → gofmt-clean Go source
tests/fixtures/{c,rust,zig}/  committed .elf + golden + layout JSON
tests/golden_test.go    table-driven golden test across all three toolchains
tests/fixtures/c/verify/  unsafe.Offsetof + bitfield round-trip verifier
examples/c-roundtrip/   demo: ELF → btf2go → cilium/ebpf integration
examples/aya-roundtrip/  same demo against Aya/Rust ELF
docs/superpowers/specs/   canonical design specs
docs/superpowers/plans/   implementation plans
docs/aya-quickstart.md    end-to-end Aya walkthrough
graphify-out/            knowledge graph artifacts (GRAPH_REPORT.md, graph.json, graph.html)
.agents/                 curated project skills
validation/              tiered validation experiment runner (see spec/plan)
```

## Current focus (volatile — refreshed by `/handoff`)

_Snapshot as of 2026-05-10 evening. May be stale; trust `git log` for ground truth._

- **Last shipped to master:** **v0.3.2** (2026-05-10). Tags: `v0.3.0` → `v0.3.1` → `v0.3.2`. Latest release: https://github.com/danigoland/btf2go/releases/tag/v0.3.2
- **In flight — v0.4 (FireLXC aya gaps):** worktree at `.claude/worktrees/feat-v04-aya-gaps` on branch `worktree-feat-v04-aya-gaps`. Spec `docs/superpowers/specs/2026-05-10-btf2go-aya-gaps-design.md` + plan `docs/superpowers/plans/2026-05-10-btf2go-aya-gaps.md` (both gitignored, both live in the worktree — recreate from master if cleaning). Five gaps closed in one release: `--aya` HashMap<K,V> unwrapping (Gap 1), `--shared-out`/`--shared-type` multi-ELF dedup (Gap 2), `--type` fallback chain + `inspect --names` (Gap 3), BTF-less diagnostic polish (Gap 4), aya-maps docs (Gap 5).
- **v0.4 progress:** Tasks 1–3 of 18 done (4 commits on worktree branch). Decoder + lookup + bridge all unit-tested (24 tests passing), reviewed (spec + code quality both ✅, one minor `containsStr→strings.Contains` nit left for CodeRabbit / cleanup pass). No PR yet.
- **Validation framework state:** all 5 automated tiers at 100% pass rate (last baseline `validation/reports/2026-05-09T07-36-14.734510896Z-ca25875-all-proxmox.md` — 24 PASS / 0 FAIL / 13 SKIP).
- **Datadog operational state:** project-scoped MCP wired, metrics + events flowing to us3, 10-widget dashboard `2n5-36z-3rc/btf2go-validation`, 3 monitors live.
- **Manual user actions outstanding (do not skip):** rotate credentials pasted in 2026-05-09 chat history — MiniMax API key, `DATADOG_API_KEY`, `DATADOG_APP_KEY`. Stored in `.env` (gitignored) but values were in conversation history.
- **Natural next step:** resume v0.4 plan execution from Task 4 (wire bridge + fallback lookup into `Resolve`). Subagent-driven workflow established; ~15 more tasks across Phases A–F then PR + CI + CodeRabbit + merge.
- **Other parked items:**
  - Real human T4 walkthrough with stopwatch (~30 min)
  - CI job for `validate.sh --tier 4` on PR merge (needs Datadog creds in CI)
  - Long-tail v0.4+ candidates: `btf.Datasec` top-level Go vars, `GoUnion.Bitfields`, `GoFile.Imports` IR refactor, `bool` typedef in quickstart, `aya_build::build_ebpf()` rename note

## Out of scope (do not propose without an issue)

- Loader / `*ebpf.CollectionSpec` generation — `bpf2go` handles this. btf2go is types-only.
- Cross-endianness output — generate on a same-endianness host as deployment target.
- Full `btf.Func` / `FuncProto` emission — Go can't represent C function signatures cleanly. PR #39 added graceful degradation: function-pointer fields render as `Pointer[uintptr]` (binary-correct 8 bytes); full signature rendering remains out of scope.
- Big-endian targets (s390x) — not tested, not supported.
