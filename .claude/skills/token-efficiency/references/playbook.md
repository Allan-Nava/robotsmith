# Playbook: where to start

**Order matters.** Caching on a workflow that does not compact intentionally saves
you 10%. Intentional compaction saves you 90%. Do not invert them.

## Week 1 — Baseline

- [ ] Instrument `usage` logging on every request (`measuring.md`).
- [ ] Count the fixed starting context: CLAUDE.md + tools + system.
- [ ] Run real tasks for a week changing nothing.
- [ ] Identify the worst task by tokens/completion and look at **where** they went.

## Week 2 — Compaction

- [ ] Adopt the QRSPI phase templates (`qrspi` skill).
- [ ] Iron rule: fresh session at every phase boundary.
- [ ] Measure the compression ratio per phase.
- [ ] Add the 40% checkpoint inside Implement.

## Week 3 — Subagents and effort

- [ ] Move all research into subagents with an imposed output format.
- [ ] Apply the per-phase effort table (`effort.md`).
- [ ] Put Haiku 4.5 on the reading subagents.

## Week 4 — Caching and hygiene

- [ ] Audit silent invalidators (`caching.md`, final table).
- [ ] Verify `cache_read_input_tokens` > 0 in Implement.
- [ ] Intermediate breakpoints in turns with many tool calls.
- [ ] Truncate tool output at the source (`tool-hygiene.md`).
- [ ] Slim CLAUDE.md below 100 lines.

## Then

Re-measure on the **same tasks** as week 1. It is the only valid comparison.

---

# Reference numbers

## Models (cached: 2026-06-24)

| Model | ID | Context | Input $/1M | Output $/1M |
|---|---|---|---|---|
| Claude Fable 5 | `claude-fable-5` | 1M | $10.00 | $50.00 |
| Claude Opus 5 | `claude-opus-5` | 1M | $5.00 | $25.00 |
| Claude Opus 4.8 | `claude-opus-4-8` | 1M | $5.00 | $25.00 |
| Claude Sonnet 5 | `claude-sonnet-5` | 1M | $3.00 | $15.00 |
| Claude Haiku 4.5 | `claude-haiku-4-5` | 200K | $1.00 | $5.00 |

Output = 5× input across the board. This is why `effort` is a bigger lever than it
looks.

## Other cost levers

- **Batch API**: 50% discount, asynchronous. Perfect for audits, mass migrations,
  backfills — anything non-interactive.
- **Cache read**: 0.1×. The biggest multiplier available, if the prefix is stable.
- **Fast mode** (`speed: "fast"`, Opus 5 / 4.8): ~2.5× output throughput, premium
  price ($10/$50). It is a *latency* lever, not a cost lever — and changing `speed`
  invalidates the cache.

## Relevant beta headers

| Feature | Header |
|---|---|
| Context editing | `context-management-2025-06-27` |
| Server-side compaction | `compact-2026-01-12` |
| Task budgets | `task-budgets-2026-03-13` |
| Cache diagnostics | `cache-diagnosis-2026-04-07` |
| Mid-conversation tool changes | `mid-conversation-tool-changes-2026-07-01` |

Mid-conversation operating instructions (`role: "system"` inside `messages[]`) do
**not** require a beta header.

## Sources

- Dexter Horthy — *Advanced Context Engineering for Coding Agents* (HumanLayer)
- [HumanLayer](https://www.humanlayer.dev/) — "Token Smarter, not Harder"
- [From RPI to QRSPI](https://alexlavaee.me/blog/from-rpi-to-qrspi/) — origin of the workflow
- [matanshavit/qrspi](https://github.com/matanshavit/qrspi) — a Claude Code implementation
- [dfrysinger/qrspi-plus](https://github.com/dfrysinger/qrspi-plus) — extension with parallel worktrees
- Claude API docs — prompt caching, context management, effort, task budgets
