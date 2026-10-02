---
title: Glossary
summary: The lab's own terms, one name per concept, and the name each page uses for the two systems under test.
type: reference
audience: [engineering, product]
covers: [versions.env, compose/compose.yaml, internal/assertion/**, internal/labspec/**, internal/journal/**, runner/lab.go, runner/env.go, runner/workspace.go, scenarios/red-by-design.txt, config/gateway/classification.yaml, victims/*/README.md]
---

# Glossary

A page uses the name in the first column and no other for the same thing. The
enforcer's own terms (verdict, decision, obligation, enforcement mode, spool,
record) are defined in `docs/concepts/glossary.md` in its repository at
`ENFORCER_COMMIT`, and this page does not repeat them.

## The systems under test

| name | meaning | not |
|---|---|---|
| the enforcer | the system that decides one tool call as it is made and records why, from `ENFORCER_REPOSITORY` at `ENFORCER_COMMIT` in [versions.env](../../versions.env); the compose service `enforcer`. A page that introduces it names the product once, "Control (the enforcer)", and says "the enforcer" after | "Guardana Control", "enforcement plane"; "plane" survives only in check names such as `plane/image` |
| the verifier | the system that grades a deployed system or a recorded run after the fact: the package `VERIFIER_PACKAGE` at `VERIFIER_VERSION`, run as the command `VERIFIER_CLI`; the compose services `verifier` and `trace-verifier`. A page that introduces it names the product once, "Guardana (the verifier)", and says "the verifier" after | "Guardana" after the introduction |
| gateway | the enforcer's MCP gateway (`ENFORCER_GATEWAY_BIN`), the process the agent talks to | a system of its own; the scenario's configuration, which is the gateway part |
| gateway part | a scenario's `gateway:` key and the files it names: the enforcer's configuration, policy and doubles' scripts for that scenario ([format](../lab-files.md#what-decides-the-run)) | "gateway" |
| system under test | the enforcer or the verifier, at its pin; read and attacked from here, never edited | |
| pin | the exact version the lab runs, in `versions.env`: the enforcer's commit, its release tag and its tree; the verifier's release; every pulled image by tag and digest. A result is evidence about that pin and nothing wider | "latest" |

## What a run is made of

| name | meaning | where |
|---|---|---|
| victim | a tool server the agent's calls reach, one per directory under `victims/` except the shared `mcpserve`; [status](../status.md) lists them | `victims/` |
| lie | the deliberately wrong annotation or description a victim carries, named in its README; never fixed, since it is what shows an annotation is a hint and never an authorization | `victims/*/README.md` |
| canary | a synthetic token planted in a victim's fixture, such as `CANARY-CRM-<id>-<hex>`, so a leak is visible in a journal or a trail; it leads nowhere | `victims/crm/`, `victims/fs/`, `victims/pay/` |
| double | a scripted stand-in for a party the enforcer talks to: `pdp-double` for the external decision point, `approver` for the person who answers a held call. Each journals what it was asked and did | `services/` |
| trajectory | the calls the scripted agent replays, the same on every run | `trajectories/<id>.yaml` |
| scenario | what a run is expected to leave on record: a verdict per step, the victims' effects, properties of the evidence | `scenarios/<class>/<id>.yaml` |
| classification | each tool's effect, resource, `trust_zone` and `returns` as the operator states them, pinned to the fingerprints the enforcer's `doctor` printed; what decides a call's effect class, never the tool's annotation | `config/gateway/` |
| profile | a compose profile a scenario boots: `core`, `enforcer`, `pdp`, `approvals`, `trace`, `chaos`, `verifier` | `compose/compose.yaml` |
| workspace | a directory outside the clone, named by `LAB_WORKSPACE` and laid out like the lab, holding scenarios of your own ([format](../lab-files.md#a-workspace-outside-the-clone)) | |
| lab key | the policy signing key `make lab-key` makes with the enforcer's own `policy keygen`, kept outside the clone, the reports and any workspace | |

## What a run leaves and how it is graded

| name | meaning | where |
|---|---|---|
| run | one scenario, booted, replayed and graded once; its id is `<scenario id>-<UTC time>-<random suffix>`, its compose project `lab-<suffix>` | `reports/<run id>/` |
| source | the file a check read, and the line in it where there is one: the evidence, a journal, `healthz.json`, `boot.json`, `probes.log`, a verifier report or pin. Every check names its source in the report's `Source` column | the run directory |
| trail | the events the enforcer records for one request, from `ACTION_PROPOSED` to the event that closes it; the enforcer calls it an evidence trail | `evidence.jsonl` |
| evidence | every event the enforcer exported for a run, decoded from the collector's export: the trails and the events that name no request. `expect.evidence` grades properties of it | `evidence.jsonl` |
| journal | the line a victim or a double appends for every call it received, served or refused, written by the server and never by the caller | `journals/<server>.jsonl`, `internal/journal/` |
| check | one graded result, named for what it establishes, such as `decisions/step-2` or `effects/victim-fs`; its outcome is pass, fail or indeterminate | `runner/check/` |
| indeterminate | the outcome of a check that established nothing, its zero value; it fails the run | `internal/assertion/` |
| `INDETERMINATE` | the enforcer's verdict for a call it could not decide; a scenario states it like any other verdict, and tolerates it on a step only by naming that step | `expect.decisions`, `tolerance` |
| catalogue | the scenarios `-all` runs: every `scenarios/<class>/<id>.yaml` under `LAB_WORKSPACE`, or under the clone when it is unset | `scenarios/` |
| suite | what a result line files a scenario under: `known-gap` for a named gap, `catalogue` for every other scenario, a workspace's included, and either with `dev-` before it against a development build | the result line |
| named gap | a scenario under `scenarios/gaps/` that asserts what a system under test does today and names, in `gap:`, the verdict it should give; it reports in the suite `known-gap` | [format](../lab-files.md#named-gaps) |
| finding | where a system under test differs from its own documentation at its pin: a decision, an evidence event or an exit code. The lab records it for the maintainers and never fixes it from here | each red-by-design line names one |
| verifier finding | an entry in the `findings` of the verifier's report, graded by `findings_include` | the verifier's report |
| red by design | a catalogue scenario that stays red on a finding, listed with the exact checks it fails on; `-red-by-design` passes only when the reds are exactly those | `scenarios/red-by-design.txt` |

## Planned

| name | meaning |
|---|---|
| Range | `planned`: running an external agent image against the same victims and grading its observed effects, with the enforcer and the verifier as optional integrations ([status](../status.md)) |
