# Measuring: baseline and instrumentation

Without a baseline you are guessing. This comes before every optimisation.

## Do not use `tiktoken`

`tiktoken` / `gpt-tokenizer` are OpenAI tokenizers. They underestimate Claude by
**15-20%** on plain text, and much more on code and non-English text. Any estimate
derived from them is wrong.

Use the official `POST /v1/messages/count_tokens` endpoint. Counts are
**model-specific**: pass the same model ID you will use at inference time.

```python
from anthropic import Anthropic

client = Anthropic()

def count(text: str, model: str = "claude-opus-5") -> int:
    return client.messages.count_tokens(
        model=model,
        messages=[{"role": "user", "content": text}],
    ).input_tokens
```

## The four `usage` fields

| Field | Meaning | Relative price |
|---|---|---|
| `input_tokens` | **only** the uncached remainder | 1× |
| `cache_read_input_tokens` | served from cache | ~0.1× |
| `cache_creation_input_tokens` | written to cache | 1.25× (5min TTL) / 2× (1h TTL) |
| `output_tokens` | generation | 5× input on Opus 5 |

> **Trap.** You see `input_tokens: 4000` after a three-hour session and conclude you
> are efficient. False: `input_tokens` is **only the uncached remainder**.
>
> ```
> total prompt = input_tokens + cache_creation_input_tokens + cache_read_input_tokens
> ```
>
> Always look at the sum.

## What to measure, in order

1. **Fixed starting context.** What CLAUDE.md + tool schemas + system prompt weigh
   before the agent does anything. Above 15k tokens you have a structural problem no
   downstream optimisation fixes.
2. **Peak context per phase.** The maximum reached during Research, Plan, Implement.
   This is the number you compare against the 40% threshold.
3. **Tokens per completed task.** Not per session — per *task*. The only metric that
   speaks to the business.
4. **Where the tokens go.** Aggregate by tool-call type. Almost always the ranking is
   (1) whole-file reads, (2) re-exploration, (3) untruncated test/build output.

## An instrumentation snippet

Wrap every call with this, in any harness:

```python
import json, pathlib

LOG = pathlib.Path(".agent-usage.jsonl")

def log_usage(phase: str, turn: int, resp) -> None:
    u = resp.usage
    total_in = (u.input_tokens
                + (u.cache_creation_input_tokens or 0)
                + (u.cache_read_input_tokens or 0))
    LOG.open("a").write(json.dumps({
        "phase": phase,
        "turn": turn,
        "input_uncached": u.input_tokens,
        "cache_write": u.cache_creation_input_tokens,
        "cache_read": u.cache_read_input_tokens,
        "output": u.output_tokens,
        "total_input": total_in,
        # 1_000_000 = Opus 5 context window
        "ctx_pct": round(100 * total_in / 1_000_000, 1),
        "cache_hit_pct": round(
            100 * (u.cache_read_input_tokens or 0) / max(total_in, 1), 1),
    }) + "\n")
```

One week of this log on real tasks is worth more than any amount of theory.
