# 05 · Implement — phase instructions

> This file is **not an artifact** to fill in: it is the Implement phase prompt.
> The output of the phase is code, plus an updated `99-progress.md`.

---

## PROMPT (fresh session per step)

> You are in the **Implement** phase of the QRSPI workflow.
>
> Input: `04-plan.md` (the section for your assigned step) + `99-progress.md`.
> **Do not read the other artifacts** unless explicitly told to: the plan is
> self-contained by construction.
>
> Assigned step: **<S1>**
>
> Rules:
> 1. Execute **only** the assigned step. If you notice something else to fix, note it
>    in `99-progress.md` under "Discoveries" — do not do it.
> 2. Follow the plan literally. If the plan is wrong or incomplete, **stop** and
>    write it in `99-progress.md` under "Deviations" — do not improvise.
> 3. Before reading a file, use `grep -n` to find the spot. Never `cat` whole files.
> 4. Run the plan's verification commands. Report the real output, not an optimistic
>    summary.
> 5. **The 40% rule:** when context passes 40%, update `99-progress.md` and stop.
>    The next session resumes from there.
>
> Recommended effort: `high` / `xhigh`.

---

## Operating loop

```
for each step S in topological order:
    ├── fresh session
    ├── input: 04-plan.md#S + 99-progress.md
    ├── implement
    ├── run the verification commands
    ├── update 99-progress.md
    ├── commit
    └── close the session
```

Parallelisable steps → separate worktrees, one session each.

---

## When to stop

Stop and write to `99-progress.md` if:

- context passes **40%**;
- the plan turns out to be wrong or incomplete;
- a test that should pass does not, and the cause is not in the current step;
- you discover a constraint `01-research.md` missed.

The last two matter most: they signal that an upstream artifact needs fixing.
Brute-forcing past them costs more than going back.

---

## Token hygiene during Implement

| Do | Don't |
|---|---|
| `grep -n "def x" -A 30 file.py` | `cat file.py` |
| `git diff --stat` then the single file | `git diff` |
| `pytest -q --tb=line` | `pytest -v` |
| `npm test 2>&1 \| tail -50` | `npm test` |
| subagent to search, with an imposed format | exploring in the main loop |

---

## Step definition of done

- [ ] Every acceptance criterion in the plan is met
- [ ] The verification commands pass (real output attached)
- [ ] No regressions on the full suite
- [ ] `99-progress.md` updated
- [ ] Commit references the step (`<TASK-ID> S1: <title>`)
