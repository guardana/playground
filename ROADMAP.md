# Roadmap

This repository is one playground for testing AI security controls; testing
agents and models is planned
([P5](#p5--agent-trials-in-the-same-playground-planned),
[P7](#p7--model-artifacts-and-endpoints-planned)). Control (the enforcer) is
the inline gate; Guardana (the verifier) grades independently. The planned
Range path will run an agent as the subject under test. Each path uses synthetic victims,
scripted scenarios and records of actual effects. The enforcer's scenarios and the
verifier's probes return a verdict without the other system; the verifier's
trace grading reads a run made through the enforcer.

Work is ordered by dependency. [Status](docs/status.md) describes what runs
today; the [failure modes](docs/reference/failure-modes.md) and
[use cases](docs/reference/use-cases.md) name the claims a new scenario must
prove.

## P0–P3 — Lab and first catalogue (implemented)

Pinned systems, deliberately deceptive victim servers, a scripted agent,
policy and approval doubles, an isolated Compose topology, the runner, the
scenario catalogue, an adopter workspace, runbooks and the quality gate. The
enforcer decides calls from its pinned commit; the verifier probes victims and
grades recorded traces from its pinned release. Several parts are still
`experimental`; [status](docs/status.md) says which, and which scenarios are
red by design.

## P4 — Public, repeatable release loop (partly implemented)

The enforcer is pinned to its public release `v0.3.0-alpha`, fetched
anonymously by commit id after its tag is checked (implemented). Each later release is taken
the same way: pin the commit, its tree and the release, rebuild both systems,
regrade every scenario and example from records, and update expectations only
from the new version's contract. A release is ready for this lab's CI when the
anonymous fetch works and the catalogue is green except for exactly the
documented red scenarios.

`make smoke` is a smaller local loop over allowed, obligation-refused, held,
trace-graded and probe-graded paths (experimental). It uses the same pins and
evidence checks; it does not replace the full catalogue. `make dev-scenarios`
builds the enforcer from a local, possibly dirty tree and grades scenarios against
it, with a report that names that tree and cannot be mistaken for a
release-pin result (experimental).

The lab's CI workflows run `make quality`, then the catalogue and the examples
against the pins (experimental). They have not run on GitHub; running them
there on every change is planned, and [status](docs/status.md) says what has
run where. Publishing a reviewed lab commit to `github.com/guardana/playground`,
which is empty today, is planned. Translate the enforcer's own scenario format only after a round trip
preserves the calls and expected evidence (planned); do not maintain two
conflicting truths for one case.

## P5 — Agent trials in the same playground (planned)

Run an external agent image by digest against the lab's synthetic victims and
tasks. The first Range slice is one local Docker profile, one normal CRM task
and one planted-canary exfiltration attempt. Record the task, agent image,
configuration, observed calls, victim effects and coverage of the tested
paths. Prove the profile's blocked direct egress and host access with negative
probes. If a path or a required record cannot be observed, the trial is
`INDETERMINATE` and fails the gate.

Start with the existing scenario runner and result semantics. Add an agent
driver only where the scripted trajectory cannot express the trial. The
verifier may grade its trace and the enforcer may gate its tool calls, independently and at
their own pins. The agent trial must still judge its own task and victim
effects when neither is installed. A separate Range repository or a generic
sandbox provider is premature until this local path works for an agent
outside the lab.

## P6 — Use-case and attack library (planned)

Add victims where they create a new observable failure, beginning with a code
host and package registry for coding agents, a tenant-partitioned retrieval
store, and a payment service that commits before a lost response. Follow with
infrastructure, calendar and authenticated MCP cases. Cover poisoned content,
tool descriptions and schemas, secret-bearing arguments, repeated calls and
denial routed around. Every case gets a deterministic trajectory, an expected
verdict, victim-side evidence and a mutation that makes its check fail.

## P7 — Model artifacts and endpoints (planned)

Serve inert model files, configurations, training code and datasets with known
defects for the verifier's supply-chain checks (`SUP-01`–`SUP-05`). Add a
model endpoint double and recorded model responses for prompt leakage and
jailbreak cases (`PRM-01`, `PRM-02`). A live model is an optional capture layer
with a stated budget and version; replay, not a live response, decides CI.

## P8 — More gates, graders and monitors (planned)

Use a driver for each other control under test: pinned build, scenario
configuration, record reader and checks against the same victim effects.
Compare the verifier's reports between pins once both have stable result records.
Add a monitor when it can be checked on a stream with known missed and false
alerts. Do not infer a pass from the absence of a finding.

## P9 — Long runs and operations (planned)

Reuse the local profiles on a test server for multiple tasks and agents, fault
injection, load and soak runs. Keep external model access and its cost bounded;
the victims remain synthetic. Publish latency or coverage numbers only with
the machine and workload that produced them. Kubernetes or another isolation
backend follows a proven local agent trial and its escape tests.
