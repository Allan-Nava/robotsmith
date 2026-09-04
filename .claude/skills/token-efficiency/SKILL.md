---
name: token-efficiency
description: Diagnose and cut token consumption in agentic coding workflows — context rot, the 40% rule, intentional compaction, subagents as context firewalls, effort and model selection, prompt caching invalidation, tool-output hygiene, KPIs. Use when asked why an agent is expensive or slow, when a session is drifting or re-exploring files it already read, when cache_read_input_tokens is unexpectedly 0, when choosing effort or model per phase, when a CLAUDE.md or prompt needs slimming, or when measuring tokens per completed task.
---

# Token efficiency for coding agents

Verified against the Claude API, reference model `claude-opus-5`.

## The mental model

With a 1M window, "does it fit?" is almost always yes. The right question is:
**how much of that context is signal?**

Agent quality degrades long before the window fills. The informal name is *context
rot*, and it shows up as: instructions given 200k tokens ago being ignored; the
agent re-exploring files it already read; mid-session decisions being contradicted;
redundant tool calls stretching the loop.

The heuristic: **keep context utilisation under ~40%.** Past that, stop and compact.

The real metric is not tokens spent:

```
signal density = useful tokens / total tokens in context
```

Cost optimisation is a *consequence* of optimising density, not the goal. An agent
working at 20% context costs less *and* errs less *and* finishes sooner. An agent at
70% costs more and produces work you have to redo — and rework is the largest, least
measured cost line.

## The levers, in order

**Order matters.** Caching on a workflow that does not compact saves you 10%.
Intentional compaction saves you 90%. Do not invert them.

| # | Lever | Typical gain | Reference |
|---|---|---|---|
| 1 | Intentional compaction (artifacts on disk between phases) | 10-20× | `references/compaction.md` |
| 2 | Subagents as context firewalls | 5-10× on research | `references/subagents.md` |
| 3 | Effort calibrated per phase | 2-3× on output tokens | `references/effort.md` |
| 4 | Prompt caching | ~10× on repeated prefixes | `references/caching.md` |
| 5 | Tool-output hygiene | 10-30k per avoided file read | `references/tool-hygiene.md` |

Before any of them: **measure**. See `references/measuring.md`.

## The five KPIs

If you track only three, track the first three.

| # | KPI | Formula | Target |
|---|---|---|---|
| 1 | Context utilisation | peak `total_input` / window, per phase | **< 40%** |
| 2 | Compression ratio | tokens burned in phase / artifact tokens | **> 15×** |
| 3 | Tokens per completed task | sum across phases, per closed task | trend ↓ |
| 4 | Cache hit ratio | `cache_read / (cache_read + input_tokens)` | **> 70%** in Implement |
| 5 | Rework rate | phases re-run / total phases | **< 15%** |

- **KPI 5 is the canary.** A high rework rate almost always correlates with a badly
  written upstream artifact, not with the model or the effort. Before raising effort,
  look at the artifact.
- **KPI 4 is read per phase**, not aggregated. In Research it is naturally low
  (always-new content); in Implement, under 70% means you have an invalidator.
- **KPI 2 under 10×** means the artifact is carrying material that should have stayed
  in the previous phase.

## References

Load only what the question needs.

| Topic | File |
|---|---|
| Baseline, `usage` fields, instrumentation | `references/measuring.md` |
| Intentional compaction, phase artifacts | `references/compaction.md` |
| Subagents, programmatic tool calling | `references/subagents.md` |
| `effort`, model choice, adaptive thinking, task budgets | `references/effort.md` |
| Prompt caching: invariant, invalidation, gotchas | `references/caching.md` |
| Tool output, context editing, server-side compaction, memory | `references/tool-hygiene.md` |
| Anti-pattern table | `references/anti-patterns.md` |
| 4-week rollout playbook, model prices, beta headers | `references/playbook.md` |

## Related

The workflow that operationalises lever #1 is the `qrspi` skill.
