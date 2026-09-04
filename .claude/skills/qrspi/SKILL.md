---
name: qrspi
description: Run a coding task through the QRSPI workflow — Questions, Research, Spec (Design + Structure), Plan, Implement — where every phase writes one self-contained artifact to disk and the next phase starts from a fresh session reading only that artifact. Use when a task is large enough that a single session would blow past 40% context, when asked to research-then-plan-then-implement, when asked to set up thoughts/ artifacts, or when a session is drifting and needs intentional compaction at a phase boundary.
---

# QRSPI

**Q**uestions → **R**esearch → **S**pec (Design + Structure) → **P**lan → **I**mplement.

Five letters, six phases: the "S" covers Design and Structure, which are separate
sessions with separate artifacts.

## The one idea

Each phase burns whatever context it needs, then compresses everything it learned
into **one markdown file on disk**. The next phase opens a fresh session and reads
only that file. Context stays under 40%, where models work best, and the artifacts
become a reviewable, diffable record of the decision.

```
Questions   ~15k burned   →  00-questions.md   (~1k)
Research    150-250k      →  01-research.md    (~5k)
Design      starts at 6k  →  02-design.md      (~4k)
Structure   starts at 9k  →  03-structure.md   (~3k)
Plan        starts at 12k →  04-plan.md        (~6k)
Implement   starts at 7k  →  code + PR
```

## Six non-negotiable rules

1. **Fresh session at every phase boundary.** Never continue.
2. **The artifact is the only channel.** If it is not written there, it does not
   exist for the next phase.
3. **Artifacts are self-contained.** Repo-root-relative paths, explicit symbols,
   line numbers. Never "the file from before", never "as discussed above".
4. **The ticket does not enter Research.** Handing the agent the ticket makes it
   hunt for evidence supporting a solution it already assumed instead of mapping
   the problem. The ticket comes back in Design.
5. **The 40% rule.** Past the threshold, stop and compact — do not push through.
6. **Do not outsource the thinking.** Every phase is a checkpoint where *you*
   correct. Approving without reading only adds latency.

## Phase table

| # | Phase | Input | Output | Effort |
|---|---|---|---|---|
| 0 | Questions | ticket | `00-questions.md` | `medium` |
| 1 | Research | `00` — **not the ticket** | `01-research.md` | `medium` + `low` subagents |
| 2 | Design | `00` + `01` + ticket | `02-design.md` | `xhigh` |
| 3 | Structure | `02` | `03-structure.md` | `high` |
| 4 | Plan | `02` + `03` | `04-plan.md` | `xhigh` / `max` |
| 5 | Implement | `04` (one step) + `99` | code + PR | `high` / `xhigh` |

Spend effort where errors propagate. A wrong Plan multiplies across the whole
implementation; a wrong Research gets caught by the Design review.

## Context budgets

Guardrails, not hard rules. Figures assume a 1M window.

| Phase | Incoming budget | Alarm threshold |
|---|---|---|
| Questions | < 20k | 40k |
| Research | — (delegate to subagents) | 400k in the main loop |
| Design | < 50k | 100k |
| Structure | < 30k | 60k |
| Plan | < 80k | 150k |
| Implement (per step) | < 100k | 400k |

Past the alarm threshold: **stop and compact.**

## On-disk layout

```
<repo>/thoughts/<task-id>-<slug>/
├── 00-questions.md
├── 01-research.md
├── 02-design.md
├── 03-structure.md
├── 04-plan.md
└── 99-progress.md
```

`<task-id>` is the Linear/Jira ID, so the artifact stays traceable. `thoughts/`
lives in the task worktree and gets committed.

## Running a phase

Load **only** the reference for the phase you are in. Each one contains the phase
prompt plus the artifact skeleton to fill.

| Phase | Reference |
|---|---|
| Questions | `references/00-questions.md` |
| Research | `references/01-research.md` |
| Design | `references/02-design.md` |
| Structure | `references/03-structure.md` |
| Plan | `references/04-plan.md` |
| Implement | `references/05-implement.md` (prompt only — the output is code) |
| Implement state | `references/99-progress.md` |

Bootstrap a task with `/qrspi:new <TASK-ID> <ticket>`, then `/qrspi:next` at each
boundary to get the next phase prompt.

## The test that matters

> An agent with **zero context** must be able to execute `04-plan.md` without
> asking a single question.

If it cannot, the plan is incomplete — and you are about to pay a rework round
that costs as much as the entire Research phase.

## Related

For the *why* — measurement, compaction ratios, subagent firewalls, effort
allocation, prompt caching — see the `token-efficiency` skill.
