# Tool-output hygiene, context editing, compaction, memory

Tool results are the #1 source of hidden context bloat. They are also the easiest to
fix.

## Do not read whole files

A `cat` of a 2000-line file is ~25k tokens, of which the agent will use 30 lines.

```bash
# ❌
cat src/services/payment.py

# ✅
grep -n "def process_refund" -A 40 src/services/payment.py
sed -n '120,180p' src/services/payment.py
```

If you are writing a harness: expose a `read` tool with mandatory `offset`/`limit`
above a certain file size.

## Truncate at the source

| Command | Instead of |
|---|---|
| `git diff --stat` then `git diff <file>` | `git diff` |
| `pytest -q --tb=line` | `pytest -v` |
| `npm test 2>&1 \| tail -50` | `npm test` |
| `docker logs --tail 100` | `docker logs` |
| `find . -name '*.ts' \| head -50` | `find .` |

Rule: if a command can produce more than ~200 lines, it needs an explicit limit
**before** you run it, not after.

## Structured output

Where you can, ask for JSON instead of prose. It is denser for the same information,
and `output_config: {format: {...}}` with `strict: true` on tools guarantees the
input validates exactly — no correction rounds.

> Note: top-level `output_format` is deprecated. Use `output_config: {format: {...}}`.
> Incompatible with citations (400).

---

## Three mechanisms people confuse

| | What it does | When | Scope |
|---|---|---|---|
| **Context editing** | **deletes** old tool results / thinking blocks | context ages over many turns | within the session |
| **Compaction** | **summarises** history into one block | you approach the window limit | within the session |
| **Memory** | persistent files the agent reads/writes | state must outlive the session | across sessions |

Many long-running agents use all three.

### Context editing

Beta `context-management-2025-06-27`. This is **cleanup**, not summarisation.

```python
client.beta.messages.create(
    model="claude-opus-5", max_tokens=4096,
    betas=["context-management-2025-06-27"],
    context_management={"edits": [{"type": "clear_tool_uses_20250919"}]},
    tools=[...], messages=[...],
)
```

Strategies: `clear_tool_uses_20250919` (with optional `clear_tool_inputs: true` to
drop the parameters too) and `clear_thinking_20251015`.

It is the natural complement to tool hygiene: hygiene limits what enters, this
removes what is no longer needed.

### Server-side compaction

Beta `compact-2026-01-12`. Automatically summarises earlier context as you approach
the threshold (default 150k tokens).

> **Critical:** append `response.content` **in full** (not just the text) to your
> messages every turn. The compaction blocks in the response are what the API uses to
> substitute the compacted history on the next request. Extracting only the text
> string silently loses the compaction state.

**It is not a substitute for intentional compaction.** It is the safety net for when
a single phase blows up anyway. The real value is still in the on-disk artifacts.

### Memory

Tool `memory_20250818`. The agent reads and writes a `/memories` directory; you
implement the backend. In a QRSPI context the phase artifacts **already are** your
memory — versioned, reviewable, diffable. The memory tool is for state that cuts
across tasks (discovered conventions, recurring codebase gotchas).
