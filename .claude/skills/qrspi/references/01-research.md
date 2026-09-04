# 01 · Research — <TASK-ID> <title>

> **PROMPT (fresh session, clean context)**
>
> You are in the **Research** phase of the QRSPI workflow.
>
> Input: `00-questions.md` (questions + answers). **The ticket text is deliberately
> withheld** — it stops you from hunting for evidence supporting a solution you
> already assumed instead of mapping the problem.
>
> Task: produce **objective facts** about the codebase.
> - Where the relevant things live (repo-root paths, symbols, line numbers)
> - Which patterns and conventions already exist
> - Which real constraints apply (DB schema, API contracts, feature flags, migrations)
> - What already exists and could be reused
> - Where the corresponding tests are
>
> **Constraints:**
> - Do NOT propose solutions. Do NOT write code. Do NOT estimate.
> - Delegate every exploration to a subagent. Impose the output format on each one:
>   `path · symbol · max 2 lines`. No pasted code in the subagent report.
> - If context passes 40%, stop exploring and write down what you have.
>
> Fill the skeleton below. It must be **self-contained**: an agent with zero context
> has to understand it. Never "the file from before", never "as seen above".
>
> Target: under 300 lines. If you overflow, you are pasting code.

This is the most expensive phase and the one with the highest compression ratio
(~30-50×). It is also the one that most needs subagents.

---

## Reference questions

From `00-questions.md` — the answers that steer this research:

- <Q1 → answer>
- <Q2 → answer>

---

## Map of the territory

### Components involved

| Area | Path | Role |
|---|---|---|
| <name> | `src/...` | <one line> |

### Entry points

| Symbol | Path:line | What it does |
|---|---|---|
| `<func>` | `src/...:120` | <one line> |

---

## Existing patterns and conventions

### <Pattern 1>

- **Where:** `src/...:45-80`
- **How it works:** <2-3 lines>
- **Who already uses it:** `src/a.py:12`, `src/b.py:200`

---

## Constraints

| Constraint | Source | Impact |
|---|---|---|
| <e.g. the `status` column is a DB enum> | `migrations/0042_*.sql:8` | <one line> |

---

## Reuse candidates

What already exists and should not be rewritten:

| What | Path | Note |
|---|---|---|
| | | |

---

## Existing tests

| What it covers | Path | How to run it |
|---|---|---|
| | | |

---

## Blind spots

Things you could not determine, and why:

- <...>

---

## Facts that contradict the assumptions

If research disproved an assumption from `00-questions.md`, write it here in large
letters. It is the most valuable output of the phase.

- <...>

---

## Status

- [ ] Research complete
- [ ] Self-contained (explicit paths, no reference to session context)
- [ ] Zero solution proposals
- [ ] Reviewed

> 📊 **Compression ratio:** <tokens burned> → <artifact tokens> = <N>×
> ⏭️ Next phase: **Design**. It receives: this file + `00-questions.md` + the ticket.
