# Stub gateway

It decides nothing.

The enforcement plane is not built yet. This stands in for it so the runner can
be proved to assert on decisions before there is anything that makes one. Every
verdict it returns was written down beforehand by the author of a scenario; this
service reads that file, replays it step by step, and records what it did. There
is no rule in it, and nothing that looks at the arguments of a call.

What it does for real:

- **Proxies the victims.** At startup it connects to every upstream in
  `LAB_UPSTREAMS`, lists their tools, and registers each one under its own name
  with the upstream's annotations carried through unchanged. A victim's lie
  about its own tool has to survive this hop, or the lab is testing the
  gateway's opinion of an annotation instead of the annotation.
- **Answers `tools/list` from the upstreams.** Every listing asks them again and
  re-registers what comes back, so a description that changed between listings
  reaches the client that asked. `victims/fs` re-describes `fs.read` on its
  second listing, and a registry filled in once at boot would answer every
  listing from the first thing it was told.
- **Forwards, or does not.** `ALLOW` and `ALLOW_WITH_OBLIGATIONS` reach the
  upstream. Anything else comes back as a result with `IsError` set, naming the
  verdict and its reason codes, and the upstream never hears about the call.
- **Writes the evidence trail** to `${LAB_REPORTS_DIR}/${LAB_RUN_ID}/evidence.jsonl`
  in the shape `internal/evidence` reads: one JSON object per line,
  `ACTION_PROPOSED` then `POLICY_DECIDED`, then `ACTION_STARTED` and
  `ACTION_COMPLETED` or `ACTION_FAILED`, or `ACTION_BLOCKED`. One `requestId`
  per call, each event linked to the one before it.

One listing is the gateway's own. It lists at startup because that is what routes
a call from an agent that never lists anything, and the scripted agent never
does, so a victim counting its own listings is one ahead of the agent: the
agent's first `tools/list` is the victim's second. A scenario that wants to watch
a description change across two of its own listings cannot get there through this
gateway today. The fix is to stop listing at startup and route the first call
through a listing instead, which moves two refusals out of startup and into the
first call: two upstreams offering one tool name, and an input schema the SDK
will not register.

The step number arrives in the call's `_meta` under
`io.guardana.playground/step` and is written into the proposed envelope's
`context.stepId` as a decimal string. That is how the runner maps a step to its
decision: from the trail, never from the agent's own log.

## Running out of declared verdicts

A step past the end of the file, or a call with no step number on it, is
answered `VERDICT_INDETERMINATE` with `STUB_NO_DECLARED_VERDICT`, and is not
forwarded. The cost is that a trajectory longer than its verdict file blocks
calls that a real gateway might have allowed. That is the direction to be wrong
in: an allow would let a run pass on steps nobody stated an answer for.

## The declared verdict file

`LAB_STUB_VERDICTS` names it, and the runner points it at the scenario's own.
Loading is strict: a key with no field is refused, because a misspelled verdict
that loads is a step the scenario's author thinks they declared.

```yaml
schema_version: 1
project_id: playground
tenant_id: tenant_a
policy_bundle_digest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
steps:
  - verdict: ALLOW
    reason_codes: [RULE_ALLOW]
  - verdict: ALLOW_WITH_OBLIGATIONS
    reason_codes: [OBLIGATIONS_ATTACHED]
    obligations: [{ type: label_sensitive }]
  - verdict: DENY
    reason_codes: [TOXIC_FLOW_SENSITIVE_TO_EXTERNAL]
```

`steps` is in trajectory order, from 1. Four of the five verdicts
`internal/labspec` declares can be written here: `ALLOW`,
`ALLOW_WITH_OBLIGATIONS`, `DENY` and `INDETERMINATE`. `REQUIRE_APPROVAL` is
refused when the file loads, because the stub has nobody to ask: it would record
`POLICY_DECIDED` and then `ACTION_BLOCKED`, a trail saying an approval was
required and never requested, which both the chain check and the decisions check
would pass over. Reason codes are copied through untouched and
checked against nothing: they come from the enforcement plane's registry, which
moves without this file, and a stub that refused a code the real plane emits
would constrain the lab to the codes it happens to know.

The default at `config/stub-gateway/verdicts.yaml` declares no steps, so a lab
brought up by hand answers every call `INDETERMINATE` and writes down that
nobody stated a verdict.

## Environment

| Variable | Meaning |
|---|---|
| `LAB_SERVER_NAME` | this service's own name, recorded as the decision's `pdpInstance` |
| `LAB_LISTEN` | address to listen on, `:8080` |
| `LAB_RUN_ID` | the run this trail belongs to |
| `LAB_REPORTS_DIR` | `/reports`, bind-mounted from the repository |
| `LAB_STUB_VERDICTS` | the declared verdict file to replay |
| `LAB_UPSTREAMS` | `victim-fs=http://victim-fs:8080/mcp,victim-web=…` |

`GET /healthz` answers once every upstream has been connected and its tools
registered, so a gateway that is up is a gateway with the tools. Compose waits
on that.

Every exchange with a victim — the handshake, a listing, a forwarded call —
carries a thirty-second bound. It is not a latency budget: the victims answer in
milliseconds. It is there so a victim that accepts a call and never answers is
recorded as `ACTION_FAILED` rather than leaving the trail at `ACTION_STARTED`,
which reads as a request still in flight.
