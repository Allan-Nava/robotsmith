# Effort and model selection

The most underrated lever on **output** tokens — which cost 5× input.

## `output_config.effort`

```python
client.messages.create(
    model="claude-opus-5",
    output_config={"effort": "medium"},   # low | medium | high | xhigh | max
    thinking={"type": "adaptive"},
    ...
)
```

Default: `high`. Lower effort means fewer preambles, more consolidated tool calls,
terser confirmations, less thinking.

## Allocation per QRSPI phase

| Phase | Effort | Model | Why |
|---|---|---|---|
| Questions | `medium` | Opus 5 | needs judgement, not depth |
| Research | `medium` | Opus 5 | it is search, not reasoning |
| ↳ research subagents | `low` | Haiku 4.5 | they read and summarise |
| Design | `xhigh` | Opus 5 | errors here cost the most downstream |
| Structure | `high` | Opus 5 | decomposition, structured work |
| Plan | `xhigh` / `max` | Opus 5 | the plan multiplies everything |
| Implement | `high` / `xhigh` | Opus 5 | quality/token sweet spot |
| Review / verification | `high` | Opus 5 | |

The principle: **spend effort where errors propagate.** A Plan error multiplies
across the whole implementation; a Research error gets caught by the Design review.

## Adaptive thinking

On all current models use `thinking: {type: "adaptive"}` — Claude decides how much to
think. `budget_tokens` is **removed** on Opus 5 / 4.8 / 4.7, Sonnet 5 and Fable 5
(returns 400). On Opus 5 thinking is **on by default**: omit the parameter and it
still runs adaptive.

Do not disable thinking to save tokens: on Opus 5 with `thinking: {type: "disabled"}`
the model can write a tool call in **visible text** instead of a `tool_use` block —
the turn succeeds, the call never fires, no error is raised. In an agent loop that
text pollutes subsequent turns. If you want to spend less, **lower the effort, do not
turn off thinking.**

## Task budget

For long agentic loops, `task_budget` gives Claude a token ceiling it is **aware
of**, so it paces itself and closes gracefully instead of being truncated (unlike
`max_tokens`, which is an imposed cut the model knows nothing about).

```python
with client.beta.messages.stream(
    model="claude-opus-5",
    max_tokens=128000,
    output_config={"effort": "high",
                   "task_budget": {"type": "tokens", "total": 64000}},
    betas=["task-budgets-2026-03-13"],
    messages=[...], tools=[...],
) as stream:
    response = stream.get_final_message()
```

Minimum `total`: 20,000. Use streaming: with large `max_tokens`, non-streaming
requests hit HTTP timeouts.
