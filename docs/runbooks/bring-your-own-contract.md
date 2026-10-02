---
title: Bring your own verifier contract
summary: Have the pinned verifier grade the agent's own trace of a run against a security contract of yours, and see the contract fail when it should.
type: runbook
audience: [engineering, product]
covers: [runner/trace.go, runner/check/verifier_report.go, agents/scripted/trace.go, compose/compose.yaml, compose/Dockerfile.verifier, config/contracts/**, examples/**]
---

# Bring your own verifier contract

The verifier at `VERIFIER_VERSION` grades the trace the lab's agent writes of a
run through the enforcer against a security contract you write in the
verifier's own format. The contract is yours; the trace is the agent's.
Trace grading is `experimental` ([status](../status.md), "Verifier loop").

## Where it goes

```
<workspace>/config/contracts/<name>.yaml
```

directly in `config/contracts/` of the workspace `LAB_WORKSPACE` names, since
the verifier is handed it by its base name. The scenario, in the same
workspace's `scenarios/<class>/`, names it and the system the contract applies to, adds the
profile `trace`, and says what the verifier must report:

```yaml
profile: [core, enforcer, approvals, trace]
trace:
  contract: config/contracts/<name>.yaml
  ai_system: <the contract's applies_to.ai_system>
expect:
  trace:
    exit_code: 0
    findings_exclude:
      - contract.<name>.<assertion id>
```

`make images` builds the verifier image; a trace run is refused when the local
image is not labelled with `VERIFIER_VERSION`.

## Its format

The verifier's contract format at the pin: `docs/usage-contracts.md` in the
verifier's repository at the tag `v<VERIFIER_VERSION>`. Of its five assertion
kinds, the lab's trace carries the evidence for two: `approval_required`,
which needs `approval` and `effects`, and `forbidden_sink`, which needs
`effects`. The agent's trace instruments `tools`, `approval` and `effects`, and
nothing for `retrieval` or `delegation`, so a `tenant_boundary`,
`allowed_scopes` or `credential_boundary` assertion makes the verifier's
analysis indeterminate, exit `2`.

## What the trace records

One span per step, in the verifier's native dialect: the call's effect is
`executed` when the victim returned a result, `attempted` when it returned an
error, and `failed` when the enforcer blocked or held it; its approval is
`not_requested`, `granted`, `timed_out`, `denied` or `unknown`. It is the
agent's own record, so state the scenario's decisions and effects beside it for
what the trace cannot tell apart
([Trace scenarios](../lab-files.md#trace-scenarios)).

Each effect names the tool as its action and the victim as its target, on the
sink a `forbidden_sink` names:

| victim | sink |
|---|---|
| `victim-crm` | `other` |
| `victim-db` | `sql` |
| `victim-fs` | `filesystem` |
| `victim-shell` | `shell` |
| `victim-mail` | `email` |
| `victim-web` | `http` |
| `victim-pay` | `payment` |

The agent refuses to record a step to a server with no sink rather than file
it under `other` (`agents/scripted/trace.go`).

## Say what you expect

`expect.trace` takes the verifier's exit code and the rules it must or must not
report, each `contract.<name>.<assertion id>`:

- `findings_include: [{rule_id: ..., severity: ...}]` for a rule that must fire;
- `findings_exclude: [...]` for a rule that must run and report nothing.

Name at least one contract rule: an exit code alone passes on a trace cut short,
whose rules all come back unverified.

## Fail it on purpose

`examples/helpdesk-payouts/` holds a contract with an `approval_required`
assertion on `crm.update_bank_account` and a `forbidden_sink` on `email`. Copy
it out of the clone and change its policy so the payout change runs without an
approval: the verifier exits 1 on
`contract.helpdesk-assistant.payout-change-is-approved-first`, and the
decisions and the victims' journals go red with it. Let the mail through
instead and `contract.helpdesk-assistant.no-email` fires. A contract that stays
satisfied under both changes is not checking what you think.
