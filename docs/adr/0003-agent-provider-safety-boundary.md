# ADR-0003: Treat model output as an untrusted rule-draft proposal

- Status: Accepted
- Date: 2026-09-03

## Context

AdFlow accepts natural-language targeting requests from an operator. A remote model is nondeterministic, may be unavailable, and may produce output influenced by prompt injection. It must not receive authority to publish campaigns or bypass domain constraints.

## Decision

Keep the model behind the existing `agentassistant.Provider` port and support two remote API styles:

1. OpenAI Responses API with strict JSON Schema Structured Outputs.
2. OpenAI-compatible Chat Completions with JSON Object output for compatible providers.

The user prompt is sent as user input, separate from stable system instructions. The system instructions explicitly treat it as untrusted data and forbid publishing, tool execution, secret disclosure, schema changes, and validation bypasses.

Every returned payload passes through these boundaries:

1. response-size limit and strict JSON decoding with unknown fields rejected;
2. Agent-specific maximum budget, impression cost, condition count, explanation, and warning limits;
3. campaign-domain targeting, budget, cost, and frequency validation;
4. human confirmation;
5. RBAC authorization for campaign publication; and
6. action audit metadata containing provider, model, prompt version, fallback state, and token count.

API keys come only from runtime configuration. The HTTP client does not follow redirects, preventing an authorization header from being forwarded to another origin. Calls have a whole-operation timeout and bounded retries only for transport failures, HTTP 408/409/429, and 5xx responses. `Retry-After` is respected up to a capped delay.

Repeated provider failures open a circuit. One half-open request probes recovery after the cooldown. When explicitly enabled, a deterministic local provider supplies a visibly marked fallback draft; the fallback never publishes.

## Consequences

- Provider failure does not grant additional authority or skip validation.
- A valid JSON response can still be rejected by business rules.
- Compatibility mode has weaker provider-enforced schema guarantees than Responses mode, so local validation remains mandatory.
- Mock fallback preserves the workflow but may produce a less expressive rule and must be shown to the operator.
- Live model quality is measured separately with an opt-in, potentially billable evaluation suite.
