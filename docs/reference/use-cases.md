---
title: Agent use cases
summary: The agent deployments the lab simulates or plans to, what each acts through, the failure modes it meets, and what the lab covers of it today.
type: reference
audience: [engineering, product]
covers: [victims/**, scenarios/**, examples/**, attacks/**, internal/labcheck/catalog_test.go]
---

# Agent use cases

The deployments below are the ones agent security tooling most often has to
hold, whoever builds that tooling. Each row names what the agent acts through,
the [failure modes](failure-modes.md) it meets, and what the lab simulates of it
today: the victims it runs, the scenarios that exercise it, and what is
planned. A team with its own deployment picks the nearest row, writes its
policy, configuration or contract for the lab's victims in a workspace
([runbooks](../runbooks/quickstart.md)), and adds a victim where none fits.
The catalogue is `experimental`.

## Acting on customers' behalf

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| Support and account servicing: answers tickets, changes accounts, issues refunds | CRM, ticketing, payments, mail | `INJ-01`, `AUTH-02`, `AUTH-03`, `TEN-01`, `FLOW-02`, `APPR-01`, `APPR-03`, `DEST-03` | victim-crm, victim-mail, victim-web; `tool-01`, `tenant-01`, `auth-01`, `approval-01`, `trace-01`, `payouts-01`. Planned: ticketing and payments victims |
| Outreach and communication: drafts and sends mail and messages, enriches contacts | mail, bulk mail, messaging, CRM | `RUN-02`, `FLOW-02`, `INJ-01`, `AUTH-01` | victim-mail; `rule-01`, `approval-02`. Planned: a messaging victim, fan-out limits |
| Personal and office assistant: reads and answers mail, books meetings, files documents | mail, calendar, files, OAuth-protected servers | `INJ-01`, `FLOW-02`, `FLOW-05`, `MCP-02`, `MCP-04`, `SEC-03` | victim-mail, victim-fs. Planned: a calendar victim, an authenticated MCP server |
| One agent serving many users through a shared service credential | any of the above, under one account | `AUTH-08`, `TEN-01`, `SEC-03` | none. Planned: a victim that holds per-user grants and records both the requester and the credential |

## Acting on company systems

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| Coding and CI: reads a repository, runs tests and commands, opens changes | files, shell, code host, package registry | `DEST-01`, `DEST-04`, `SEC-01`, `SEC-04`, `SUP-03`, `INJ-01`, `INJ-03`, `RUN-01` | victim-fs, victim-shell; `rule-02`, `tool-03`, `mode-02`. Planned: a code host and a package registry victim |
| Data and analytics: queries warehouses, writes reports | SQL, files, mail | `DEST-01`, `TEN-01`, `FLOW-02`, `RUN-03` | victim-db, whose read tools accept writes; `pdp-03`. Planned: a destructive statement through a read tool, a query cost bound |
| Finance operations: pays invoices, changes payout details, reconciles | payments, CRM, ledger | `DEST-03`, `APPR-01`, `APPR-02`, `APPR-03`, `APPR-04`, `AUTH-03`, `FLOW-04` | victim-crm's payout change: `tool-01`, `trace-01`, `trace-02`, `payouts-01`; approvals held on a send: `approval-01`..`05`; `obligation-01`. Planned: a payments victim that charges before it times out, amount caps, retries |
| IT operations: runs runbooks, changes infrastructure, answers incidents | shell, cloud and cluster APIs, monitoring | `DEST-01`, `DEST-02`, `AUTH-05`, `AVAIL-04`, `APPR-01` | victim-shell; `mode-02`. Planned: an infrastructure victim with a production and a test environment |
| Long-running jobs and agents started by events or schedules | queued tools, webhooks, schedulers | `DEST-05`, `AUTH-09`, `AVAIL-03`, `AVAIL-05` | the slow and silent victims of `chaos-01` and `chaos-02`. Planned: a job victim that commits late, events with an origin |

## Working with knowledge

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| Research and browsing: fetches pages, searches, summarises | web fetch, search | `INJ-01`, `INJ-02`, `FLOW-03`, `RUN-03` | victim-web and the injected pages in `attacks/`; `flow-01`, `flow-02`, `auth-01`. Planned: data leaving in a fetched URL |
| Knowledge assistant over company documents | a retrieval store, documents | `TEN-03`, `INJ-02`, `INJ-04`, `FLOW-02`, `FLOW-05` | none. Planned: a retrieval victim with tenant partitions and a poisoned document |
| Regulated records: health, HR or legal files, with consent and retention | a records system, scheduling | `TEN-01`, `FLOW-02`, `EVID-01`, `SEC-03` | the tenancy and evidence scenarios, `tenant-01` and `evidence-01`, on other victims. Planned: a records victim with consent scopes |

## Agents acting through screens and speech

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| Browser and desktop agents that read a screen and click or type | a browser or a desktop session | `UI-01`, `INJ-01`, `FLOW-05`, `FLOW-06` | none. Planned: a screen victim that changes under the agent and records the target each action reached |
| Voice agents taking spoken commands | a speech stream, then any tool | `AUTH-02`, `SEC-03`, `FLOW-05` | none. Planned: needs a recorded model |

## Agents working with agents

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| A planner delegating to workers, or one agent handing off to another | delegation, agent-to-agent calls | `AUTH-05`, `SEC-02`, `SEC-03`, `RUN-01`, `RUN-05` | none: delegation cannot be configured at the enforcer's pin. Planned: once the enforcer takes delegation chains |

## The things agents are built from

| use case | acts through | failure modes | in the lab today |
|---|---|---|---|
| Building and training models: fine-tuning, a model registry, notebooks, datasets | model files, training scripts, notebooks, datasets | `SUP-01`, `SUP-02`, `SUP-03`, `SUP-04`, `SUP-05` | none. Planned: inert artifacts the verifier scans, each planted with one known defect |
| Publishing an MCP server for agents | the server's own listing and authentication | `DRIFT-01`, `DRIFT-02`, `INJ-03`, `MCP-01`, `MCP-02`, `MCP-03` | victim-fs, whose tool changes on its second listing; `verify-01`..`04`, `chaos-04`. Planned: an authenticated server |
| Serving a model endpoint to an agent or a user | prompts, answers | `PRM-01`, `PRM-02`, `RUN-03` | none. Planned: needs a model, recorded once and replayed |

## Across every use case

Availability under faults (`AVAIL-01`..`05`), the evidence a deployment keeps
(`EVID-01`..`03`) and runaway behaviour (`RUN-01`..`05`) belong to no single
deployment. The chaos, spool and mode scenarios hold the first two against the
victims above; runaway behaviour is planned, and a monitor under test would
read the same runs.

## Other products

The gate the lab runs today is the enforcer at `ENFORCER_COMMIT` and the grader
is the verifier at `VERIFIER_VERSION`. Every victim, trajectory, payload and
record reader is theirs to share: a victim's journal and a trajectory say
nothing about which gate stood between them. Running another gate or grader
against the same scenarios needs a driver for it: how it is built, configured
per scenario and read back. That is `planned` in the roadmap.
