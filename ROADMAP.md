# Roadmap

Ordered by dependency, not by date. `docs/status.md` says what exists today;
[docs/reference/failure-modes.md](docs/reference/failure-modes.md) and
[docs/reference/use-cases.md](docs/reference/use-cases.md) say which failure
modes and deployments each phase adds.

## P0 — Rules and gate (implemented)

Repository rules, hygiene guards, the quality gate, pinned versions of the
systems under test.

## P1 — Skeleton, victims, scripted agent (implemented)

Six tool servers with synthetic data and deliberately wrong annotations, a
scripted agent that replays a trajectory as real tool calls, a runner that
grades from records. The stub gateway it started with is retired.

## P2 — The real enforcer, the verifier and the first catalogue (implemented)

Scenarios decided by the enforcer built from its pinned commit and graded from
its exported trail: rules, tenancy, approvals, the external decision point,
obligations, modes, a full evidence spool, chaos on the victims' paths. The
verifier at its pinned release probes the victims and grades the agent's own
trace against a security contract.

## P3 — Open to strangers (implemented)

A workspace outside the clone for a team's own policy, gateway configuration
and contract; a worked example; the runbooks; the lab on a Linux host as an
ordinary user; a CI job that builds both systems from their pins and judges the
catalogue against the scenarios red by design; the failure-mode and use-case
catalogues every scenario maps to. The CI job waits on the enforcer's commit
being fetchable from its public repository.

## P4 — The use-case library (planned)

Victims and scenarios for the deployments the use-case catalogue marks
`planned`, each victim with its own deliberate lie, ordered by how often the
deployment is met and what a gate has to decide there:

- payments that charge before they time out, amount caps, retries that must not
  charge twice (`DEST-03`, `FLOW-04`);
- a code host and a package registry for coding agents: secrets in arguments,
  untrusted packages, poisoned repository content (`SEC-01`, `SUP-03`,
  `INJ-03`);
- a retrieval store with tenant partitions and a poisoned document (`TEN-03`,
  `INJ-02`, `INJ-04`);
- an infrastructure victim with a production and a test environment
  (`DEST-02`);
- a calendar and an authenticated MCP server (`MCP-02`, `MCP-03`);
- loops, fan-out and a denial routed around (`RUN-01`, `RUN-02`, `RUN-05`).

## P5 — The model supply chain (planned)

Inert model files, configurations, notebooks, training scripts and datasets,
each planted with one known defect, served by a registry victim an agent
fetches from; the verifier's scans graded against them (`SUP-01`..`05`).

## P6 — Other gates, graders and monitors (planned)

A driver per kind of tooling, so another MCP gateway or policy engine, another
trace grader or scanner, or a monitor runs against the same victims,
trajectories and payloads: how it is built and pinned, how a scenario
configures it, and how its records are read into the decisions, findings and
detections the checks grade. The enforcer's detectors join here once they
ship.

## P7 — Live models and benchmarks (planned)

A live model as an overlay, recorded once and replayed, never the thing that
decides a build (`PRM-01`, `PRM-02`, richer `INJ-01`); decision latency against
policy size, published with the machine that produced it.
