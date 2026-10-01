---
title: Bring your own gateway configuration
summary: Which settings of the enforcer's gateway a scenario sets, which the lab owns and refuses, and how a run assembles the rest.
type: runbook
audience: [engineering, product]
covers: [runner/gateway/**, runner/enforcer.go, runner/env.go, compose/compose.yaml, config/gateway/**, examples/**]
---

# Bring your own gateway configuration

Your part of the enforcer's configuration: its mode, who the agent is, how it
holds approvals, how its evidence spool behaves, its timeouts. The runner adds
everything that ties the enforcer into the lab and refuses a part that sets any
of it. Running the enforcer in the lab is `experimental` (`docs/status.md`,
"Enforcer in the lab").

## Where it goes

```
<workspace>/config/gateway/scenarios/<name>.yaml
```

in the workspace `LAB_WORKSPACE` names, named by the scenario's
`gateway.config`, next to its policy
([bring your own policy](bring-your-own-policy.md)). The format is the
enforcer's configuration file at the pin, `docs/reference/configuration.md` in
its repository at `ENFORCER_COMMIT`. The runner reads your part as YAML and
writes the assembled file in the block style the enforcer's parser requires.
At the pin, `mode`, `project_id`, `tenant_id`, `listener.principal.id` and
`listener.agent.id` have no default, so your part sets them.

## What a scenario sets

Only these keys, each where the enforcer's configuration puts it
(`runner/gateway/allow.go` holds the list):

| group | keys |
|---|---|
| top level | `mode`, `project_id`, `tenant_id`, `environment` |
| `log` | `level` |
| `listener` | `kind`, `principal.id`, `principal.type`, `principal.tenant_id`, `agent.id`, `agent.framework`, `agent.version` |
| `policy` | `max_stale`, `fail_open_read` |
| `pdp` | `timeout`, `max_in_flight` |
| `approvals` | `provider`, `ttl`, `retry_after`, `max_held`, `max_open`, `max_records`, `max_record_bytes`, `reconcile_max` |
| `evidence` | `max_bytes`, `segment_bytes`, `closing_reserve`, `fsync`, `fsync_interval`, `on_unwritable` |
| `list` | `shaping`, `ttl` |
| `upstream` | `call_timeout`, `list_timeout` |

Any other key is refused before anything boots, with the list of what is
allowed; so is a key spelled with a dot (`a.b: x`), which the enforcer would
read as `a: {b: x}`.

## What the lab owns

The runner writes the rest into the run's own copy of the configuration,
`reports/<run id>/gateway/gateway.yaml`, which you can read after the run:

- the listener's address: the enforcer's own address on the run's `agent-net`,
  never a wildcard, so no container on the enforcer's other networks reaches it;
- the health address the runner reads `/healthz` and `/brand` from;
- the signed bundle and the lab key's public half;
- the evidence spool's directory and the export to the lab's collector;
- the six victims as upstreams, with their tenants and environment;
- the classification of every victim tool, pinned to its fingerprint;
- the decision point's identifier, when the scenario names a script for the
  double (`gateway.pdp_script`, profile `pdp`), and the approvals directory the
  approver answers in (`gateway.approver_script`, profile `approvals`); the
  two scripts are file names in the workspace's `config/pdp/` and
  `config/approver/`.

## Try a setting

`examples/helpdesk-payouts/config/gateway/scenarios/helpdesk-assistant.yaml`
sets a mode, a principal, an agent and file approvals with their `ttl` and
`retry_after`. Change one there, in a copy outside the clone, and say in the
scenario what the enforcer's docs at the pin say that setting changes: a
shorter `approvals.ttl` expires a hold the approver answers too late, a smaller
`evidence.max_bytes` blocks a call before its effect, `mode: OBSERVE` runs what
`ENFORCE` blocks. `docs/lab-files.md` says how a step states what a held call,
a resumed one or a blocked one leaves in the trail.

## Fail it on purpose

Change the setting back, or to a value the enforcer's docs say decides
otherwise, and keep the expectation: the run goes red on the records that
setting changes. A setting whose change leaves the run green is one the
scenario does not observe.
