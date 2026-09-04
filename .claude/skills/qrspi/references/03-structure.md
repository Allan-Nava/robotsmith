# 03 · Structure — <TASK-ID> <title>

> **PROMPT (fresh session, clean context)**
>
> You are in the **Structure** phase of the QRSPI workflow.
>
> Input: `02-design.md`. Nothing else.
>
> Task: decompose the design into steps that are implementable and **independently
> verifiable**.
>
> Constraints for every step:
> - it has an objective done-criterion **verifiable with a command**;
> - it leaves the repo in a working state (tests green) when complete;
> - it fits in one session under 40% context — if it does not, split it;
> - it declares its dependencies on other steps.
>
> **The test:** if you cannot say *how a step is verified*, it is not a step — it is
> an intention. Rewrite it.
>
> Mark which steps can run in parallel on separate worktrees.
>
> Do NOT write the detailed plan: that is the Plan phase. Decomposition only.
>
> Recommended effort: `high`.

---

## Reference

Design: [`02-design.md`](./02-design.md)

---

## Steps

### S1 · <title>

- **Goal:** <one line>
- **Touches:** `src/...`, `tests/...`
- **Depends on:** — (none)
- **Verify:** `pytest tests/test_x.py -q` passes
- **Repo state after:** working, feature not yet exposed

### S2 · <title>

- **Goal:**
- **Touches:**
- **Depends on:** S1
- **Verify:**
- **Repo state after:**

### S3 · <title>

- **Goal:**
- **Touches:**
- **Depends on:** S1
- **Verify:**
- **Repo state after:**

---

## Dependency graph

```
S1 ──┬── S2 ──── S4
     └── S3 ──────┘
```

**Parallelisable:** S2 and S3 (separate worktrees, no shared files).

---

## Recommended execution order

1. S1
2. S2 ‖ S3
3. S4

---

## Per-step risks

| Step | Risk | Fallback |
|---|---|---|
| | | |

---

## Status

- [ ] Decomposition complete
- [ ] Every step has a verification command
- [ ] Every step leaves the repo working
- [ ] Dependencies and parallelism mapped

> ⏭️ Next phase: **Plan**. It receives: this file + `02-design.md`.
