---
title: Failure modes
summary: How an agent deployment goes wrong, which kind of tooling must catch each failure, and the scenarios that simulate it today.
type: reference
audience: [engineering, product]
covers: [scenarios/**, examples/**, internal/labcheck/catalog_test.go]
---

# Failure modes

Every scenario names the failure modes it is an instance of in
`maps_to.failure_catalog`, and `internal/labcheck` holds this page and the
scenarios to each other: an identifier no row defines, a scenario no row lists
or a row listing a scenario that does not map to it fails the build.

The catch column names the tooling that has to stop or find the failure,
whoever builds it: a **gate** decides each tool call as it is made, a
**grader** checks after the fact, a **monitor** watches the stream of actions.
Here the gate is the enforcer and the grader the verifier, each at its pin; no
monitor is under test yet. [use-cases.md](use-cases.md) maps deployments to
modes. A scenario file states what it expects per step, benign calls
included, and [lab-files.md](../lab-files.md) says which record each
expectation is read from.

The catalogue is `experimental`. In the last column, `mode-01` and `verify-04`
are red by design (`scenarios/red-by-design.txt`), and `payouts-01` is the
worked example in `examples/helpdesk-payouts/`, which `make scenarios` does not
run.

## Authority

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `AUTH-01` | a call outside what the principal is granted runs | gate | `rule-01`, `rule-02`, `pdp-01` |
| `AUTH-02` | content or a caller claims authority it does not have | gate | `auth-01` |
| `AUTH-03` | a tool's self-description decides instead of the operator's classification | gate | `tool-01` |
| `AUTH-04` | a tool nobody classified is called | gate | `tool-03`, `mode-01` |
| `AUTH-05` | a delegated or handed-off agent gets more authority than its parent | gate, grader | planned: delegation cannot be configured at the enforcer's pin |
| `AUTH-06` | an external decision point is ignored, or asked without the facts it needs | gate | `pdp-01`, `pdp-03` |
| `AUTH-07` | the effect that lands differs from the call that was authorized: an alias, a redirect, a read that writes | gate, grader | planned |
| `AUTH-08` | a shared service credential succeeds where the requesting user holds no grant | gate, grader | planned |
| `AUTH-09` | an event or a schedule starts work with authority its originator lacks | gate | planned |

## Tenancy

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `TEN-01` | a call reaches another tenant's resource | gate | `tenant-01` |
| `TEN-02` | a material call names a tenant on one side only | gate | `tenant-03` |
| `TEN-03` | retrieval returns another tenant's documents | gate, grader | planned: needs a retrieval victim |

## Injection

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `INJ-01` | fetched content asks for a call, and the call is let through | gate, grader | `auth-01`, `flow-01`, `flow-02` |
| `INJ-02` | a tool result carries instructions into the next step | grader | planned |
| `INJ-03` | a tool's schema or description hides instructions | grader | planned |
| `INJ-04` | poisoned memory steers a later run | grader | planned |

## Data flow

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `FLOW-01` | untrusted content, then a private read, then an external send | gate, grader | `flow-01`, `flow-02`, `flow-03`, `gaps-02` |
| `FLOW-02` | data reaches a sink the deployment forbids | gate, grader | `flow-01`, `flow-03`, `trace-01`, `trace-02`, `payouts-01` |
| `FLOW-03` | data leaves inside a request to an attacker's address | gate | planned |
| `FLOW-04` | a redaction or a cap the policy requires is not applied before the effect | gate | `obligation-01` |
| `FLOW-05` | private data reaches the agent's answer rather than a tool | grader, monitor | planned: needs the answer recorded |
| `FLOW-06` | data is split or encoded across calls that each look harmless | gate, grader, monitor | planned |

## Approvals

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `APPR-01` | a material action runs without an approval | gate, grader | `approval-01`, `approval-04`, `trace-01`, `trace-02`, `payouts-01` |
| `APPR-02` | one approval is spent twice | gate | `approval-02` |
| `APPR-03` | an action changed after its approval runs on it | gate | `approval-03` |
| `APPR-04` | an approval nobody answered in time still lets the action run | gate | `approval-05` |
| `APPR-05` | a held action outlives a restart in an unknown state | gate | planned |

## Destructive and irreversible actions

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `DEST-01` | a destructive command runs through a general-purpose tool | gate | `rule-02`, `mode-02` |
| `DEST-02` | a change meant for a test environment reaches production | gate | planned |
| `DEST-03` | a payment exceeds its cap, or runs twice on a retry | gate | planned: needs a payments victim |
| `DEST-04` | code or a workflow the agent wrote runs with CI credentials, or merges unreviewed | gate, grader | planned |
| `DEST-05` | a long-running job commits after its timeout or cancellation, and a retry runs it again | gate | planned |

## Evidence

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `EVID-01` | an action runs and no record of it is kept | gate, grader | `evidence-01`, `evidence-02`, `tool-02` |
| `EVID-02` | the record says authorized bytes ran when none were authorized | gate | `mode-01` |
| `EVID-03` | records are lost between the decision and the store they go to | gate, grader | `chaos-03` |

## Availability

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `AVAIL-01` | a decision point that is slow, down or unreadable lets calls through | gate | `pdp-02` |
| `AVAIL-02` | a stale policy keeps deciding | gate | `rule-03` |
| `AVAIL-03` | a slow or silent tool leaves a call in an unknown state | gate | `chaos-01`, `chaos-02` |
| `AVAIL-04` | an operating mode enforces otherwise than its documentation says | gate | `mode-01`, `mode-02` |
| `AVAIL-05` | concurrent calls pass a shared limit, or the target changes between decision and effect | gate | planned |

## Drift

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `DRIFT-01` | a tool's definition changes after it was approved | gate, grader | `chaos-04`, `verify-01`, `verify-04` |
| `DRIFT-02` | a stable tool is reported drifted, or an unpinned one clean | grader | `verify-02`, `verify-03` |

## Runaway behaviour

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `RUN-01` | the agent repeats the same call without end | monitor, grader | planned |
| `RUN-02` | one instruction fans out to many recipients or records | gate, monitor | planned |
| `RUN-03` | calls, tokens or cost grow without a bound | grader, monitor | planned |
| `RUN-04` | the agent reports success for an action that failed or never ran | grader, monitor | planned |
| `RUN-05` | after a denial the agent reaches the same effect another way | grader, monitor | planned |

## Secrets and identity

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `SEC-01` | a secret travels in a tool argument | gate, grader | planned |
| `SEC-02` | a credential is passed across a trust boundary | grader | planned |
| `SEC-03` | a session stands in for an identity, or two records disagree on who acted | grader | planned |
| `SEC-04` | a secret a tool returned is disclosed later in the run | grader, monitor | planned |

## MCP servers

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `MCP-01` | an MCP server the deployment requires to authenticate answers without it | grader | planned |
| `MCP-02` | a token for another audience, or too broad a scope, is accepted | grader | planned: needs an authenticated victim |
| `MCP-03` | a session is not bound to the client that opened it | grader | planned |
| `MCP-04` | a server's elicitation or sampling request extracts private input or induces an action | gate, grader | planned |
| `UI-01` | a click or keystroke lands on another target than the one decided | gate, grader | planned: needs a screen victim |

## Model supply chain

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `SUP-01` | a model file executes code when it is loaded | grader | planned |
| `SUP-02` | a model's configuration or loader fetches and runs remote code | grader | planned |
| `SUP-03` | a training script or notebook imports a package that should not be trusted | grader | planned |
| `SUP-04` | a dataset differs from the one its record names | grader | planned |
| `SUP-05` | a secret is written into a pipeline artifact | grader | planned |

## Model endpoints

| id | what goes wrong | catch | in the lab today |
|---|---|---|---|
| `PRM-01` | a jailbreak, direct or gradual, gets past the model's policy | grader | planned: needs a model |
| `PRM-02` | the system prompt or a secret leaks in an answer | grader | planned: needs a model |
