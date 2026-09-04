# Prompt caching

## The single invariant

> **Caching is a prefix match. One byte changing at position N invalidates
> everything from N onward.**

Render order: **`tools` → `system` → `messages`**.

Everything else follows from this. Get the ordering right and caching works almost by
itself. Get it wrong and no amount of `cache_control` markers saves you.

## Design rules

- **Stable content first, volatile content last.** Always.
- **Never** `datetime.now()`, session IDs, user IDs, or conditional sections in the
  system prompt. They sit at the head of the prefix and invalidate everything below.
- **Serialise tools in deterministic order** (sort by name). A `json.dumps()` without
  `sort_keys=True`, or iterating a `set`, is enough to break everything.
- **Do not change the tool set mid-session.** Tools render at position 0. If you need
  a "mode", pass it as message content, not by swapping tools.
- Max **4** breakpoints per request.

## Minimum cacheable length (not monotonic!)

| Model | Minimum |
|---|---:|
| Opus 5, Fable 5, Mythos 5 | **512** |
| Opus 4.8, Sonnet 5, Sonnet 4.6, Sonnet 4.5 | 1024 |
| Opus 4.7 | 2048 |
| Opus 4.6, Opus 4.5, Haiku 4.5 | **4096** |

A 3k-token prompt caches on Opus 5 and **silently does not cache** on Opus 4.6 or
Haiku 4.5. No error — just `cache_creation_input_tokens: 0`.

## Mid-conversation operating instructions

If you need to inject an instruction mid-session (mode change, dynamic state), **do
not edit the top-level `system`** — that invalidates the entire cached history.
Append a message with `role: "system"` inside `messages[]` instead:

```python
system=[{"type": "text", "text": CORE, "cache_control": {"type": "ephemeral"}}],
messages=[
    *history,
    {"role": "user", "content": "..."},
    {"role": "system", "content": "Terse mode: answers under 40 words."},
]
```

It sits **after** the history, so the cached prefix stays intact. It is also the
unforgeable operator channel (unlike a `<system-reminder>` inside a user turn, which
anyone writing to user input can forge).

Available on Opus 5, Opus 4.8, Fable 5, Mythos 5. **Not on Sonnet 5** — there it
returns 400; fall back to a text block in the user turn.

Constraints: it must follow a `user` message, cannot be `messages[0]`, and must be
the last element or be followed by an `assistant` turn.

## Two agent-loop-specific gotchas

**The 20-block lookback window.** Each breakpoint walks backwards **at most 20
content blocks** to find a previous cache entry. An agentic turn with many
`tool_use`/`tool_result` pairs easily exceeds 20 blocks: the next breakpoint finds no
cache and **silently misses**.
→ Fix: place an intermediate breakpoint every ~15 blocks in long turns.

**Concurrent requests.** A cache entry becomes readable only once the first response
**starts streaming**. N parallel requests with the same prefix all pay full price.
→ Fix: make one serial warm-up request before fanning out.

## Invalidation hierarchy

Not everything invalidates everything:

| Change | tools cache | system cache | messages cache |
|---|:---:|:---:|:---:|
| Tool definitions (add/remove/reorder) | ❌ | ❌ | ❌ |
| Model change | ❌ | ❌ | ❌ |
| `speed`, web-search, citations | ✅ | ❌ | ❌ |
| System prompt content | ✅ | ❌ | ❌ |
| `tool_choice`, images, thinking on/off | ✅ | ✅ | ❌ |
| Message content | ✅ | ✅ | ❌ |

Useful implication: you can change `tool_choice` or toggle thinking per request
**without** losing the tools+system cache. Only tool changes and model changes force
a full rebuild.

## Economics

- Cache read: **~0.1×**
- Cache write: **1.25×** (5 min TTL) / **2×** (1h TTL)
- Break-even: 5-min TTL pays off from **2 requests** (1.25 + 0.1 = 1.35 vs 2);
  1h TTL from **3** (2 + 0.2 = 2.2 vs 3).

The 1-hour TTL keeps entries alive across gaps in bursty traffic, but the doubled
write needs more reads to pay for itself. For continuous agentic sessions the default
TTL is fine; for a team resuming a task after an hour-long meeting, use 1h.

## Verification

If `cache_read_input_tokens` is **0** on repeated requests with a theoretically
identical prefix, there is a silent invalidator. Diff the rendered prompt bytes
between two requests. The usual suspects, by frequency:

| Pattern | Why it breaks |
|---|---|
| `datetime.now()` in the system prompt | different prefix every request |
| `uuid4()` / request ID at the head | same |
| `json.dumps(d)` without `sort_keys=True` | non-deterministic serialisation |
| session/user ID interpolated into system | per-user prefix, zero sharing |
| `if flag: system += ...` | every flag combination is a distinct prefix |
| `tools=build_tools(user)` | tools are at position 0 |
