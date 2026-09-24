# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Fixed

- `make enforcer-image` builds again from a fresh clone: the tool lister a
  chaos scenario runs inside a victim is its own command, `relist`, and
  `compose/healthprobe`, which the enforcer image builds without the lab's
  module, is held to the standard library by a test.

### Changed

- `expect.effects` accounts for every journal line of the run, not only the
  served ones: an entry takes an optional `calls_refused: {tool: n}` beside
  `calls_served`, and a line whose status and tool the scenario does not name,
  or whose status the lab does not know, fails the effects check with the line
  quoted. The victims' journals and both doubles' are graded alike. The format
  stays `schema_version: 1`; a scenario that names no refusal asserts none.
- `versions.env` pins the tree of the enforcer's commit as `ENFORCER_TREE`.
  `make enforcer-image` refuses a commit whose tree is not it and labels the
  image `io.guardana.playground.enforcer.tree` with the tree it verified;
  `plane/image` fails unless the enforcer container's own image carries that
  label equal to the pin, and says which tree it found. An enforcer image
  built before this change carries no such label and has to be rebuilt.
- The gate's file list reads git only in the repository's own work tree and
  refuses an empty list, so a copy inside another repository is scanned file by
  file and no guard reports clean having read nothing.
- `versions.env` pins the enforcer by commit (`ENFORCER_COMMIT`) and the
  verifier at 0.26.1, and every image the lab pulls by tag and the digest of
  its multi-arch index. `ENFORCER_TAG`, `ENFORCER_BRAND_ENDPOINT` and the Postgres,
  Jaeger and OPA images are gone until something reads them, and the collector
  and toxiproxy images came back with the services that read them; a test in
  `compose/` fails on a variable nothing reads.
- The verifier image is built by `make verifier-image` alone: the `verifier`
  service carries no `build:` and the runner builds nothing before a verifier
  or trace run, so the image it checked against `VERIFIER_VERSION` is the one
  that runs.

### Added

- Chaos: a scenario the enforcer decides can name `chaos:` faults, applied
  after boot and lifted before the drain, each graded as `chaos/fault-<n>` from
  a record showing it in place: a `latency` or `hang` toxic on one victim's
  answers through `toxiproxy-tools` (profile `chaos`, pinned as
  `TOXIPROXY_IMAGE`, alone on `tool-net`, its API on loopback), held to the
  trail (a hang closes `RESULT_STATUS_TIMEOUT` at the scenario's
  `upstream.call_timeout`); `collector: down`, lifted once compose reports the
  collector running and the enforcer's exporter acknowledges records again;
  and `relist`, a second listing of a victim's tools from inside it
  (`relist`) that describes a tool otherwise than its listing
  snapshot. A trajectory step can say `on_error: continue` to carry on past a
  JSON-RPC error the gateway answered, never past a call no gateway answered,
  and not in a scenario with `trace:`. `chaos-01`..`04` grade a slow victim, a
  victim that never answers, a collector outage and a tool description changed
  under the running gateway.
- A catalogue against the enforcer at its pinned commit: rules and a stale
  policy, an unclassified tool, tenancy, approvals held, resumed, reused,
  mutated, rejected and expired, the external decision point and its failures,
  obligations, `OBSERVE` and `LOCKDOWN`, a full spool, and the toxic-flow gap.
  `mode-01` is red by design, on a contradiction in the enforcer recorded as a
  finding.
- Scenarios can grade the decision point double's and the approver's journals
  in `expect.effects`, and the decision point a decision names
  (`pdp_instance`, `none` or an identifier). A run the enforcer decides is
  graded on the enforcement mode every event records and on the executed digest
  of every completion. A catalogue check refuses a scenario naming a double's
  script that does not exist.
- The trail order follows the enforcer's state diagram: an approval that
  expired is followed by a new request or a block, never by a run.
- Trace scenarios: a trajectory scenario can name `trace: {contract, ai_system}`
  and `expect.trace`, graded like one verifier step that names at least one
  `contract.` rule. The scripted agent writes its own record of the run
  (`-trace`) in the verifier's native trace dialect, with each call's effect and
  approval (`unknown` while held, `granted`, `denied` or `timed_out` once the
  hold ends, `not_requested` when never held; an upstream's error is an
  `attempted` effect), and the pinned verifier's
  `analyze-trace` grades it against a contract in `config/contracts/`. It runs
  as the `trace-verifier` service (profile `trace`), alone on the internal
  `trace-net`, with the run's `verifier/` directory and the contracts mounted
  read-only. `trace-01` and `trace-02` in `scenarios/trace/` show the contract
  holding for an approved payout change and a refused shell command, and broken
  by a policy that lets the change run unapproved.
- The runner refuses a trace or verifier run when the local verifier image is
  not labelled with `VERIFIER_VERSION`, and compose never pulls it. What a
  container printed is kept only under a path that did not exist, each stream
  is bounded, and the agent's trace and a verifier's pin are read only as
  regular files.
- Every upstream the enforcer fronts carries the scenario's `environment`, which
  the enforcer requires to decide a write, a delete or a configuration change.
- Verifier scenarios: a scenario with `verifier` probe steps instead of a
  trajectory, graded on each step's exit code, the pin it wrote and the JSON
  report it printed (`expect.verifier`), and on every victim's journal. It runs
  in the `verifier` profile alone. The `verifier` compose service runs the
  pinned verifier on `tool-net` alone, hardened, with only the run's
  `verifier/` directory mounted; a run fails when the verifier has a route out
  or a default route. Four scenarios in `scenarios/verify/` probe the victims
  at guardana 0.26.1; `verify-04` is red while the verifier reports drift at
  another severity than its rule catalogue lists.

- `make enforcer-image` builds the enforcer from `git archive` of its pinned
  commit in the clone `ENFORCER_SOURCE` names, never from a working tree, and
  refuses when the archive does not hash to that commit's tree or the clone
  carries replacement refs. `make verifier-image` installs the verifier from a
  hash-locked requirement file, then removes pip and every setuid or setgid bit.
- `compose/otel/collector.yaml` and a `collector` service (profile `enforcer`,
  network `evidence-net`, an internal network no other service is on) run the
  pinned OpenTelemetry Collector with only its file exporter (`append: true`,
  so a restart does not truncate what it already wrote), so the enforcer's
  OTLP export lands somewhere the runner can read it and nothing else on the
  lab's networks can reach. `runner/otlp.go` decodes that file with
  `internal/evidence.DecodeOTLP` and writes it out as the run's
  `evidence.jsonl`, one event per line, refusing an empty or eventless export
  rather than writing an empty one; a decode error never leaves a partial
  trail in its place. Neither is wired into a scenario run yet.
- `internal/evidence` reads the enforcer's trail from its OTLP/HTTP JSON log
  export: the body of each record is the event, its attributes must agree, a
  redelivered event collapses, and each trail is ordered by its links within one
  request, project and tenant. The mirror carries the three fields the contract
  gained (`prevEventDigest`, `decidedAt`, `redactionProfile`).
- `services/pdp-double`, an AuthZEN decision point double for the enforcer's
  external decision point: HTTPS only, a CA made in memory and scoped by
  critical name constraints to its own names, answers scripted per scenario,
  hostile by default, and every question journalled before it is answered.
- The scenario format pairs steps with the trails they open by order, so a run
  can be graded against a gateway that writes no step number or run id. A step
  can `resumes` a held trail (stating `trail` or `blocked`, never a verdict) or
  open none (`opens: none`), and grade the trail's `ACTION_BLOCKED` (`blocked`)
  and its whole sequence of kinds (`trail`). Each trail's proposal must name the
  tool its step calls. A trajectory step can `wait_before` its call and
  `retry_while_pending`, which retries only an answer the enforcer's gateway
  marks as its own under `ENFORCER_NAMESPACE`. Unchanged files load and grade
  as before.
- Named gaps: scenarios under `scenarios/gaps/` carry `gap: {wanted, why}`,
  assert what the system documents today and run as their own suite, named in
  the runner's summary and in every JUnit classname (`known-gap.<id>` or
  `catalogue.<id>`).
- Every scenario run gets its own compose project, lab-built images are named
  through `LAB_IMAGE_PREFIX`, and the agent's image is rebuilt before every run
  so a fixed name never replays a stale agent. A trail that names no run is
  read as the run's own only from the directory the runner just created.
- `services/approver` answers held approvals the way a person would, through
  the enforcer's own `approvals list|approve|reject`, per a scenario script:
  approve, reject or leave, after a delay; never while no plane holds the
  directory, never an unreadable record unless a rule names it. A command cut
  short is journalled as unknown until the next listing shows what the record
  says. `compose/Dockerfile.approver` puts it beside the pinned `/enforcer/control`.
- Documentation machinery: page frontmatter, budgets and `covers` in
  `docs/docs.json`, checked by `make docs-frontmatter` (outside the gate until
  the pages carry it); a generated `docs/README.md`; `make docs-impact` for
  the pages a change makes suspect. The link check no longer reads untracked
  run reports.
- Scenarios the enforcer decides: a `gateway:` block names the scenario's part
  of the enforcer's configuration and its policy; the runner signs the policy
  for the run with the lab key (`make lab-key`, made once per machine outside
  the clone and the reports by the enforcer's own keygen), assembles the
  configuration around it from the keys a scenario may set (a dotted key is
  refused), boots the enforcer from its pinned image with the collector, reads
  the trail after the spool drained with nothing quarantined, truncated,
  refused or dropped and the collector flushed, and fails a run whose enforcer
  does not report the pinned commit or whose container does not run the pinned
  image. `gateway.upstream_tenants` puts a victim in a tenant of its own. The
  stub moves to its own compose profile. `tool-02` is the first such scenario.
- The lab's classification of every victim tool (`config/gateway/`) is pinned
  to the fingerprints the enforcer's own doctor prints (`make
  classify-victims`), and those to the listing snapshots they were taken from
  (`tools.sha256`); a changed definition or a snapshot rewritten alone fails a
  test without Docker.
- Each service mounts only the part of the run it writes (the victims, the
  double and the approver `journals/`, the agent `agent/`), no longer the whole
  reports directory. The agent's log moves to `agent/agent.jsonl`.
- Compose services for the decision point double and the approver, each on a
  network it shares with the enforcer alone or with nothing.
- Every run report opens with its provenance: the lab commit (untracked files
  count as uncommitted changes), each pin, each image on this machine with its
  ID and the pin its label names, and the machine.
- The runner refuses to start when the environment sets a variable
  `versions.env` pins, since compose would use that value instead of the pin.
- Repository rules, hygiene guards and the quality gate CI runs.
- Pinned versions of the systems under test in `versions.env`.
- Trajectory and scenario file formats in `internal/labspec`, with the evidence,
  journal and assertion readers a run is graded from.
- Compose topology in `compose/`: ten services on two networks, with `tool-net`
  marked internal so nothing in the lab has a route out, and the stub gateway as
  the only service on both.
- Stub gateway in `services/stub-gateway`: it replays declared verdicts and
  writes the evidence trail. A step nobody declared is answered
  `STUB_NO_DECLARED_VERDICT`, so a scenario cannot pass on the stub's silence.
- Six victim tool servers — crm, db, fs, shell, mail and web — and the
  `victims/mcpserve` package they share. Each carries the wrong annotations its
  README names, and each records the calls it served in a journal a scenario is
  graded from.
- Scripted agent in `agents/scripted`: it replays a trajectory as real tool
  calls and forwards each step's output into the next, so a toxic flow is a real
  data flow.
- Scenario runner, `make scenario ID=...` and `make scenarios`: it boots the
  profile, proves the agent cannot reach a victim except through the gateway,
  replays the trajectory, and grades the run from the evidence trail, the
  victims' journals and what came up. Checks for boot, topology, replay,
  decisions, effects and evidence, with JUnit and Markdown reports.
- Attack payload catalogue in `attacks/`: four indirect prompt injections served
  by `attacker-web`.
- Three scenarios with their trajectories and declared verdicts:
  `tool-01-permitted-read-is-recorded`, `flow-01-injected-page-to-external-mail`
  and `auth-01-cross-tenant-export-undecided`.

Nothing is released yet. The scenarios run end to end against the stub gateway,
and the verdicts they replay come from a file rather than from anything that
decides.
