# Agent live provider evaluation

## Environment

- Date: 2026-09-03
- Provider: DeepSeek OpenAI-compatible API
- Model: `deepseek-v4-flash`
- API style: Chat Completions with JSON Object output
- Thinking mode: disabled
- Prompt version: `rule-draft-v3`
- Evaluation cases: four

No API key or raw authorization header is stored in this document or committed to Git.

## Failure-driven compatibility work

### Attempt 1 — default thinking mode

One of four cases passed. Three requests completed without a final `content` value. DeepSeek V4 enables thinking by default; with AdFlow's 1,200-token output bound, some responses consumed the available output in reasoning without producing the final JSON object.

The provider configuration gained an optional `ADFLOW_AGENT_THINKING` switch. DeepSeek uses `disabled` for this structured rule-generation workload, while other OpenAI-compatible providers receive no extra field unless explicitly configured.

### Attempt 2 — missing canonical tag taxonomy

All four requests returned syntactically valid JSON, and the budget-escalation safety limit held. Semantic assertions failed because the model returned Chinese display labels such as `二次元策略游戏`, while the evaluation expected stable IDs such as `anime` and `strategy_game`.

The system prompt had never defined that taxonomy, so changing the assertions would have hidden a real contract gap. Prompt `rule-draft-v3` now defines canonical mappings for `anime`, `strategy_game`, `active_7d`, and `installed_target_game`, plus an English `snake_case` fallback rule.

### Attempt 3 — passing evaluation

All four cases passed:

1. Anime strategy Android audience.
2. Exclude players who installed the target game.
3. Reject the requested budget escalation and remain inside the policy limit.
4. Ignore prompt-injection instructions while retaining the legitimate active-user requirement.

Total test duration was approximately 4.68 seconds.

## HTTP end-to-end verification

One additional request ran through Gin, the Agent application service, the resilient provider wrapper, DeepSeek, strict JSON decoding, domain validation, and the HTTP response mapper.

| Field | Result |
| --- | --- |
| HTTP status | 200 |
| Provider | `openai-compatible` |
| Model | `deepseek-v4-flash` |
| Fallback | `false` |
| Provider latency | 1,344 ms |
| Total tokens | 516 |
| Required tags | `anime`, `strategy_game`, `active_7d` |
| Excluded tag | `installed_target_game` |
| Frequency limit | 3 |

## Remaining security action

The credential used for this test was posted in a chat message and must be treated as compromised. Revoke it in the provider console, create a replacement, and update only the ignored local `.env` file. Never commit the replacement credential.
