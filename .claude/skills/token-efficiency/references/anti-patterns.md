# Anti-patterns

| Anti-pattern | Typical cost | Fix |
|---|---|---|
| 500-line CLAUDE.md | ~8k tokens on **every** session | < 100 lines; the rest in on-demand skills |
| Continuing the same session across phases | 5-10× on the downstream phase | fresh session + artifact |
| Relying on auto-compact | context rot + rework | intentional compaction before 40% |
| "Read the whole codebase and then…" | 200k+ | a subagent that returns a map |
| Tool set that varies per user/mode | cache hit ~0% | fixed tool set + tool search |
| Timestamp or mode in the system prompt | cache hit ~0% | trailing `role: "system"` message |
| `cat` of whole files | 10-30k per file | `grep -n` + `sed -n` |
| Subagent with no imposed output format | cancels the benefit | explicit format in the prompt |
| Vague plan → Research redone | 250k repeated | the plan must be executable at zero context |
| Original ticket passed to Research | solution-first thinking | the ticket returns in Design |
| Cold parallel fan-out | all requests pay full price | serial warm-up |
| Disabling thinking to save tokens | phantom tool calls in the text | lower the `effort` instead |

## The CLAUDE.md multiplier

Worth a note of its own. Every line of CLAUDE.md is paid for:

```
cost = lines × tokens/line × sessions/day × days
```

100 superfluous lines × 15 tokens × 40 sessions/day = **60k tokens/day** of pure
noise, which also eats instruction budget.

The fix is the same idea as QRSPI applied to instructions: **the bare minimum always
in context, everything else loaded on demand** (skills, with only the `description`
resident).
