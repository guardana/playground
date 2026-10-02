---
title: Bring your own policy
summary: Run a policy of your own through the pinned enforcer against the lab's victims, and see what it allows, denies and holds.
type: runbook
audience: [engineering, product]
covers: [runner/workspace.go, runner/enforcer.go, runner/signing.go, runner/gateway/**, config/gateway/classification.yaml, examples/**]
---

# Bring your own policy

A policy document in the enforcer's own format, signed for each run with the
lab key and loaded by the enforcer at `ENFORCER_COMMIT`. The lab never edits
it. `examples/helpdesk-payouts/` is a worked one. Running the enforcer in the
lab is `experimental` ([status](../status.md), "Enforcer in the lab").

## Where it goes

In a workspace: a directory outside the clone, laid out like the lab, that
`LAB_WORKSPACE` names
([docs/lab-files.md](../lab-files.md#a-workspace-outside-the-clone)):

```
<workspace>/config/policies/<name>.json
<workspace>/config/gateway/scenarios/<name>.yaml
<workspace>/trajectories/<id>.yaml
<workspace>/scenarios/<class>/<id>.yaml
```

The scenario names it:

```yaml
gateway:
  config: config/gateway/scenarios/<name>.yaml
  policy: config/policies/<name>.json
```

and you run it with `LAB_WORKSPACE=<workspace> make scenario ID=<id>`.

## Its format

The enforcer's policy document format at the pin: `docs/reference/policy-format.md`
in the enforcer's repository at `ENFORCER_COMMIT`, with
`docs/guides/write-and-test-a-policy.md` beside it. Read them at that commit,
not at the enforcer's latest: the lab runs that commit and nothing else. Check
a document before a run with the enforcer's own linter, from the image
`make images` built, `<ENFORCER_COMMIT>` being the full commit id in
`versions.env`:

```
docker run --rm --network none -v "<workspace>/config/policies:/p:ro" \
  --entrypoint /enforcer/control playground-enforcer:<ENFORCER_COMMIT> \
  policy lint /p/<name>.json
```

## What your rules see

The enforcer builds each call's envelope from its own configuration, and the
lab owns most of it:

- **The principal and the agent** are the listener's, set in your gateway part
  (`listener.principal`, `listener.agent`). The listener authenticates nobody,
  so nothing a call carries changes who makes it, and the runner refuses a
  trajectory that names another principal, tenant, agent or environment.
- **The action** is the tool's name, `crm.read_customer`, and its effect class,
  resource type, resource id and destination trust zone come from the lab's
  classification of its victims, `config/gateway/classification.yaml`, pinned
  to each tool's fingerprint. You cannot reclassify a victim's tool; a scenario
  can only leave one unclassified on purpose (`gateway.unclassified`).
- **The run's flow state**, which a `flow.toxicAtLeast` rule reads, comes from
  each tool's `trust_zone` and `returns` in that same file, per tool and never
  per path ([what decides the run](../lab-files.md#what-decides-the-run)).
- **Tenants**: the principal's is `listener.principal.tenant_id` in your part
  (left unset, the enforcer at the pin fills in the gateway's `tenant_id`). A
  victim has no tenant unless the scenario names one in
  `gateway.upstream_tenants`, and a material call with a tenant on one side
  only is `INDETERMINATE`. `pay.charge`, `pay.refund` and `pay.payout` are
  `TRANSACT`, which at the pin requires a tenant on both sides
  (`docs/contracts.md` in the enforcer's repository at `ENFORCER_COMMIT`), so a
  scenario that calls them names `victim-pay` there.

The victims, their tools and what each lies about are in `victims/*/README.md`.

## Say what you expect

Each step of the scenario states the verdict and reason codes the enforcer's
docs at the pin say your policy produces, and the trail kinds its call leaves:

```yaml
expect:
  decisions:
    1:
      verdict: ALLOW
      reason_codes_include: [RULE_ALLOW]
      trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_COMPLETED]
  effects:
    victim-crm: { calls_served: { crm.read_customer: 1 } }
```

Reason codes are the enforcer's (`docs/reference/reason-codes.md` at the pin);
the lab invents none. `effects` is exhaustive: every call a victim received is
named, served or refused, so a denial reads as a victim that served nothing.

## Fail it on purpose

Change the policy, never the expectation, and run it again: turn an `ALLOW`
rule into `DENY`. The run goes red on the decision the rule made and on the
journal of the victim that no longer served the call. A scenario that stays
green is not checking what you think.
