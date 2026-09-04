# 02 · Design — <TASK-ID> <title>

> **PROMPT (fresh session, clean context)**
>
> You are in the **Design** phase of the QRSPI workflow.
>
> Input: `00-questions.md`, `01-research.md`, and **now also the original ticket**.
>
> Task: propose the solution design, anchored to the facts in `01-research.md`.
> - Every claim about the codebase must cite a path from the research doc.
> - If the design needs a fact research does not cover, **say so explicitly** in the
>   "More research needed" section instead of inventing it.
> - Present rejected alternatives with the reason. The review needs them.
>
> This document will be **commented on by humans**. Write it to be criticised:
> explicit decisions, explicit trade-offs, explicit weak spots.
>
> Recommended effort: `xhigh`.
>
> Target: under 250 lines.

This is the main human checkpoint. Correcting 200 lines of markdown costs
infinitely less than correcting 2000 lines of wrong code.

---

## Problem

<2-4 lines. The problem, not the solution.>

**Ticket:** <ENG-1234> · <url>

---

## Proposed solution

<Prose description, 1-2 paragraphs. Details below.>

### Diagram

```
<ASCII or mermaid — only if it adds something>
```

### Components to touch

| Component | Path | Kind of change |
|---|---|---|
| <name> | `src/...` | new / modify / remove |

---

## Decisions

### D1 · <decision>

- **Choice:** <what>
- **Why:** <reason, anchored to a fact in `01-research.md`>
- **Rejected alternatives:**
  - <alternative> — rejected because <reason>
- **Reversible?** yes / no — <if no, why it matters>

### D2 · <...>

---

## Impact

| Area | Impact | Mitigation |
|---|---|---|
| DB schema | | |
| Public API | | |
| Performance | | |
| Security | | |
| Data migration | | |
| Backward compat | | |

---

## What we are NOT doing

Explicitly excluded scope, with the reason:

- <...>

---

## More research needed

Facts the design assumes but `01-research.md` did not verify:

- [ ] <...>

> If this list is not empty, consider a short targeted Research round **before**
> moving to Structure. It costs less than the rework.

---

## Review

| Comment | From | Status | Resolution |
|---|---|---|---|
| | | open / resolved | |

---

## Status

- [ ] Design written
- [ ] Anchored to research facts (every claim has a path)
- [ ] Alternatives documented
- [ ] Reviewed by the team
- [ ] Comments resolved
- [ ] Approved

> ⏭️ Next phase: **Structure**. It receives: this file only.
