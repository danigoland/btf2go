---
title: T4 multi-model sweep — comparison report
generated: 2026-05-10
runs_analyzed: 5
---

# T4 multi-model sweep — comparison

Five fresh-user T4 walkthroughs across three models and two environments, conducted to test how reliably the spec H4 hypothesis ("≤30 min end-to-end Aya + btf2go integration") holds against different model + harness combinations.

## Run matrix

| # | Model | Env | Method | Status | Wall-clock | Friction reported | Artifacts verified | PR |
|---|---|---|---|---|---|---|---|---|
| 1 | MiniMax M2.7 | Daytona | opencode agentic loop | success | 5.9 min | 2 (encountered) | pass | [#60](https://github.com/danigoland/btf2go/pull/60) |
| 2 | Kimi K2.6 | Daytona | INFRA_BLOCKED → orchestrator manual | blocked | 20 min¹ | 5 | pass² | [#61](https://github.com/danigoland/btf2go/pull/61) |
| 3 | Qwen3-Coder-Next | Daytona | INFRA_BLOCKED → orchestrator manual | partial | 13 min¹ | 3 | partial | [#59](https://github.com/danigoland/btf2go/pull/59) |
| 4 | Kimi K2.6 | Proxmox VM | direct `/api/generate` one-shot | success | 1.7 min | 6 (predicted) | pass | [#62](https://github.com/danigoland/btf2go/pull/62) |
| 5 | Qwen3-Coder-Next | Proxmox VM | python+Ollama agentic, 10 turns | success | 4.6 min | 6 (encountered) | pass | [#63](https://github.com/danigoland/btf2go/pull/63) |

¹ Includes infra-setup time before the orchestrator gave up on the inner-agent path.
² Verified by orchestrator manually executing the walkthrough; not by the LLM.

## Methodology caveats

The five runs are **not directly comparable** — they used three distinct methodologies that produce different signal types:

- **Agentic loop** (#1, #5): the model issues a command, sees real stdout/stderr, decides next action. Friction is *encountered*. Most representative of fresh-user behavior.
- **One-shot prediction** (#4): the model writes the full walkthrough as a single inference, predicting commands and outputs. Friction is *predicted* — the model imagines what would surprise a fresh user. Suspicious to call this "fresh-user simulation."
- **Orchestrator-manual fallback** (#2, #3): the inner-agent harness was infra-blocked; the (Sonnet) orchestrator ran the walkthrough itself. Useful for surfacing real friction but the agent isn't the model under test.

**Only #1 and #5 are directly comparable as fresh-user simulations.**

## H4 hypothesis status

> **H4: A new user can integrate btf2go end-to-end into a fresh Aya project in ≤30 min using only README + docs/aya-quickstart.md.**

| Run | Wall-clock | H4 met? |
|---|---|---|
| #1 M2.7 agentic | 5.9 min | ✓ |
| #5 Qwen3 agentic | 4.6 min | ✓ |

Both fresh-user simulations completed comfortably under the 30-min budget. **Lower-bound evidence supports H4** — but with the caveat below.

**Important honest caveat:** an LLM's "self-conducted" walkthrough is at best a *lower bound* on real-user difficulty. LLMs don't:
- Get distracted, stop, ask Slack, or call lunch
- Misread error messages and rabbit-hole on the wrong fix
- Hesitate before running unfamiliar commands

The strict friction-aware prompt mitigated some of this (M2.7 logged a `u5` Rust hallucination, then recovered; Qwen3 needed 10 turns including 3 backtrack-on-error cycles). But the absolute confidence is "at least one competent agentic LLM finished in 5-6 min" — not "a typical human will finish in 5-6 min."

A real human walkthrough by the project owner is still the H4 spec's authoritative evaluator.

## Real btf2go bugs surfaced (actionable)

Eight distinct findings worth landing as fixes / docs:

### High-severity (silent / misleading)

1. **Rust/Aya silently emits BTF-less ELF when bpf-linker LLVM version mismatches** (run #2, #5). bpf-linker v0.10.3 wants LLVM-22; many environments ship LLVM-19. Build *succeeds* but the resulting `.elf` has no `.BTF` section. Subsequent `btf2go inspect` finds nothing and the user has no signal pointing at the linker. **Fix candidates:** detect missing `.BTF` and emit a hard error in `btf2go inspect`; quickstart should document version-check command.
2. **Standalone Aya without `aya-build` produces no `.BTF`** (run #3). The quickstart's example assumes the user already has `aya-build` configured; users following along with a bare `cargo init` get a structless ELF.
3. **`btf2go generate` is silent on success** (run #1). No `Generated: <path>` line. Fresh users couldn't tell it worked. **Fix:** print one stderr line on success.

### Documentation gaps

4. **Bitfield accessors are C-only** (run #1). The agent hallucinated `u5` (Rust has no arbitrary-width integer types). Quickstart should clarify: bitfield Get/Set methods are emitted only when the C source uses bitfield syntax; equivalent Rust patterns aren't generated.
5. **`#[tracepoint(name = "...")]` macro syntax fails** (run #3). Aya 0.13's macro is bare `#[tracepoint]` — quickstart shouldn't show the named-attribute form.
6. **`asm/types.h` missing on Debian trixie** when compiling C-side BPF programs (run #5). Default `clang -target bpf` sysroot doesn't include `/usr/include/asm/`. Workaround: include path tweak OR self-contained typedefs. Quickstart should call this out.
7. **`btf2go generate` auto-discovery requires explicit `--type`** when struct is a map value (run #5). Without it, output is just the `Pointer[T any]` declaration and the user is confused.

### CLI ergonomics

8. **`btf2go --version` returns "unknown flag"** (run #1). Add a `version` subcommand or `--version` flag.

## Cross-model insights

- **M2.7 (run #1)** was the cleanest agentic run — 4 turns, 5.9 min. It hit the `u5` hallucination and `btf2go --version` and recovered both. With more aggressive friction-prompting it might surface more — the original 2.3-min run reported only 2 friction; this run reported 2 too even with the strict prompt. M2.7's pattern: optimistic, decent recovery, low friction self-report.
- **Kimi K2.6 (run #4)** can't drive an opencode agentic loop in non-interactive mode (opencode v0.0.55 + anomalyco v1.14.46 both lack non-interactive Ollama provider support). Direct `/api/generate` one-shot worked and the prediction was largely correct, but **Kimi got 2 layout details factually wrong** — predicted `_pad` (unexported) where btf2go actually emits `Pad` (exported); missed a trailing `_pad0` alignment field. This is a meaningful prediction-vs-reality gap in Kimi's btf2go knowledge.
- **Qwen3-Coder-Next (run #5)** with the custom python+Ollama harness behaved most like a real user — 10 turns, 4.6 min, 6 surfaced friction items including 3 backtrack-on-error cycles. Non-thinking mode (1-3s/call) made each turn fast; verbose prose around the bash blocks hallucinated checkmarks before commands ran (caught by the harness). Qwen3 produces the highest-fidelity friction signal of the three models.

## Net effect on the validation framework

The sweep found **8 actionable btf2go items** the framework's automated tiers (T1-T2.5, T3) would never have found, because they're all about the human-side first-experience surface:
- T2 cares about layout correctness, not "is `--version` a flag"
- T3 builds Aya projects via known-good build commands, not via the quickstart's prose

T4 — even with LLM-driven runs — is the only tier that exercises this surface. The friction-aware prompt protocol from this sweep is the recommended T4 mode going forward.

## Recommended next moves

1. **Fix the 8 items** — most are 1-line docs PRs; the BTF-less-ELF detector in `btf2go inspect` is the biggest (a 10-20 line change).
2. **Make T4 multi-model the standard.** Re-run on master after each material doc/CLI change. Three runs (M2.7 + Kimi + Qwen3) takes ~10 min total parallelized; cost is dominated by infra setup.
3. **Real human walkthrough** by the project owner remains the H4 spec's gold-standard evaluator. The sweep is a credible *lower bound*; humans surface friction LLMs paper over.
4. **Drop opencode for Ollama-Cloud T4 runs.** Custom python+Ollama harness (run #5) was strictly better — it actually agentically loops where opencode doesn't.

## Methodology gotchas worth recording

- **Daytona sandboxes block `ollama.com`** at the TLS layer (Cloudflare, IP `34.36.133.15`). All other major SaaS endpoints work. Use Proxmox VMs or LXC for any Ollama-Cloud-driven testing.
- **`daytona exec` reproducibly strips first line of stdin.** Workaround: prefix with `echo DUMMY &&`. File a bug with Daytona.
- **Daytona's API requires `X-Daytona-Organization-ID` header** that neither the CLI nor MCP currently sets. Both return 401 unless the header is added by direct HTTP.
- **opencode (v0.0.55, anomalyco v1.14.46) lacks non-interactive Ollama support.** Direct `/api/generate` is the workaround for scripted Ollama Cloud runs.
- **Reverse-tunnel approach** (`ssh -N -R 11434:localhost:11434 dani@<vm>`) lets a Proxmox VM use the host's already-signed-in Ollama daemon. Works for N parallel VMs sharing one host Ollama, no auth-token transfer.
