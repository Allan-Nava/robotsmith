# Intentional compaction — the 10-20× lever

The heart of QRSPI, and on its own worth more than every other optimisation combined.

## Auto-compact vs intentional compaction

| | Auto-compact | Intentional compaction |
|---|---|---|
| **When** | when the window is nearly full | at a phase boundary, decided by you |
| **What it keeps** | the model decides | you decide, in writing |
| **Where it ends up** | in context, ephemeral | on disk, versioned in git |
| **Reviewable** | no | yes, by humans and other agents |
| **Ratio** | ~2-3× | **15-30×** |

Auto-compact is *blind* and *late* compression: it fires when the context-rot damage
is already done, and it picks what to throw away.

## The pattern

**Every phase writes an artifact to disk. The next phase restarts from zero context
reading only that artifact.**

```
Questions   →  ~15k burned     →  00-questions.md   (~1k)
Research    →  150-250k        →  01-research.md    (~5k)
Design      →  starts at 6k    →  02-design.md      (~4k)
Structure   →  starts at 9k    →  03-structure.md   (~3k)
Plan        →  starts at 12k   →  04-plan.md        (~6k)
Implement   →  starts at 7k    →  code + PR
```

The point is not only cost. It is that **Implement runs steadily under 20% context**
— the zone where the model performs best. Without intentional compaction, Implement
would start from 250k tokens of Research residue and end past 50%.

## Operating rules

1. **One artifact per phase, on disk, in git.** A task worktree is exactly the right
   place: the artifact lives on the task branch.
2. **Never continue the same session across two phases.** Fresh session, clean
   context, and as input only the previous phase's artifact (plus, if needed, one
   from further upstream — but explicitly, not "everything").
3. **The artifact must be self-contained.** Repo-root-relative paths, explicit symbol
   names, line numbers. Never "the file we looked at earlier", never "as discussed
   above".
4. **The plan quality test:** an agent with **zero context** must be able to execute
   it. If it needs to ask for clarification, the plan is incomplete and you are about
   to pay a rework round.
5. **Intra-phase compaction.** This also applies *inside* Implement on long tasks: at
   ~40%, stop, have it write a `99-progress.md` (done / todo / unexpected discoveries
   / decisions taken), close the session, reopen from there.

## Ticket context isolation

A subtle but important QRSPI detail: **the original ticket text is not passed to
Research.** The reason is to avoid *solution-first thinking* — if the Linear ticket
says "add a field to the users table", the agent goes looking for evidence supporting
that solution instead of mapping the problem.

Research must produce objective facts about the codebase. The ticket comes back in
Design, once the facts are on the table.

## Why not one giant prompt

QRSPI's predecessor was RPI (Research, Plan, Implement): three monolithic prompts of
85+ instructions each. The measured problem is **instruction budget overflow**: an
LLM reliably follows on the order of 150-200 instructions. An 85-instruction prompt,
plus CLAUDE.md, plus tool schemas, plus MCP servers, saturates the budget before the
task even arrives.

Short phases are not only about compressing tokens: they are about **staying inside
the instruction budget**.
