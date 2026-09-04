# Subagents as context firewalls

The second multiplier.

A searching subagent burns 100k tokens and returns 2k to the parent. **The parent's
context stays clean.** Same idea as compaction, applied horizontally instead of
across time.

## The rule

> **Subagents read. The main loop decides and writes.**

| Delegate to a subagent | Keep in the main loop |
|---|---|
| "find where X is implemented" | architectural decisions |
| directory and naming-convention exploration | writing code |
| reading logs, dumps, verbose output | the plan and its state |
| independent parallel verifications | anything needed *afterwards* |
| audits along separate dimensions (security, perf) | synthesising the audits |

## Three details people get wrong

**1. The subagent prompt must impose short, structured output.**
A subagent that dumps 20k tokens on the parent has cancelled the benefit. State the
format explicitly:

```
Return ONLY:
- file path (relative to repo root)
- symbol / function
- max 2 lines of explanation
No preamble, no pasted code.
```

**2. A fork that rebuilds the prompt loses the parent's cache.**
If the subagent regenerates `system`, `tools`, or uses a different `model`, it reads
nothing from the parent's cache. For reuse: copy `system`/`tools`/`model`
**verbatim** and append only the fork-specific content at the end.

**3. This is also the correct way to use a cheap model.**
Caches are model-scoped: switching models mid-session invalidates everything. Do not
downgrade the main loop — put Haiku 4.5 in a reading subagent and leave the main loop
on Opus 5.

## Programmatic tool calling

A more aggressive variant of the same principle. Instead of N round trips (Claude
calls → result enters context → Claude reasons → calls again), Claude writes a
**script** that invokes the tools as functions inside the code execution container.
Intermediate results stay in the script; **only the final output enters context**.

Use it when you have sequential chains of calls with large intermediate results that
get filtered anyway.

Setup: declare `{"type": "code_execution_20260120", "name": "code_execution"}` and
set `"allowed_callers": ["code_execution_20260120"]` on your custom tool.
Incompatible with `strict: true`, `disable_parallel_tool_use`, and forced
`tool_choice`.
