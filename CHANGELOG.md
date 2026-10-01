# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

- A development mode for the enforcer: `make dev-scenarios
  CONTROL=<checkout> [ID=<scenario>]` builds it from a checkout's working
  tree, uncommitted changes included, as `playground-enforcer-dev:<tree>`
  (`scripts/build-enforcer-dev.sh`), and runs the catalogue or one scenario
  with `runner -enforcer-dev <image>`. The run signs policies with that build
  and is held to its version, its tree and the image ID read at the start;
  every result line and JUnit case reads `dev-<suite>`, and the report names
  the checkout, its HEAD, whether it was dirty, the tree, and the pinned
  commit the run did not use. Compose takes the enforcer's image from
  `LAB_ENFORCER_REF`, which the runner always sets, and tags the approver per
  enforcer image (`LAB_ENFORCER_TAG`), so a development run never reuses the
  pinned run's approver or the other way round.
- A failure-mode catalogue (`docs/reference/failure-modes.md`) and a use-case
  catalogue (`docs/reference/use-cases.md`), written for any gate, grader or
  monitor, not only the two systems under test here. Every scenario names its
  modes in `maps_to.failure_catalog`, which now resolves: a test fails on an
  identifier no row defines, a scenario no row lists, or a row listing a
  scenario that does not map to it. `ROADMAP.md` is rewritten around the
  catalogue's planned rows.
- Runbooks for a stranger and an adopting team: `docs/runbooks/quickstart.md`
  (from `git clone` to one green scenario, reading the report, what a red
  run means, cleaning up) and one page each for bringing your own policy,
  gateway configuration and verifier contract. `README.md` is rewritten
  around them; it no longer says no scenario runs.
- A CI scenario job (`.github/workflows/scenarios.yml`): the enforcer's commit
  fetched by its id from `ENFORCER_REPOSITORY` (new in `versions.env`) by
  `scripts/fetch-enforcer.sh`, which fails with the reason and never skips,
  then `scripts/ci-scenarios.sh` (`make ci-scenarios`): both images, a lab key
  of its own, the catalogue judged by `runner -all -red-by-design
  scenarios/red-by-design.txt`, and every example from a copy outside the
  clone. The list names each red scenario with the finding that keeps it red;
  a listed scenario that passes, an unlisted red or a listed id that no longer
  exists fails the run by name.
- Three checks on the enforcer's agent listener, each recorded in
  `probes.log`: from the first victim the trajectory calls, and the decision
  point double when its profile is up, `listener-closed-to/<service>` (a
  refused connection at the enforcer's address on their network) and
  `agent-address-unreachable-from/<service>` (no route or no answer at its
  agent-net address); and `listener-bound-to-agent-net`, from the address the
  enforcer says its listener bound. A boot that fails says why in
  `boot.json`.
- `expect.health` grades the enforcer's own `/healthz` counters after the
  replay, each count exact: `blocks` by reason code, `reads_unrecorded`,
  `sink_failures_before_effect`. The answer is kept in the run directory as
  `healthz.json`; a missing or unreadable one fails every stated count.
  `evidence-01` and `evidence-02` now assert the cause of their blocks and of
  the read run unrecorded, which no trail records.
- `LAB_WORKSPACE=<dir>` runs your own scenarios, trajectories, policies,
  gateway parts, contracts and double scripts from a directory outside the
  clone, laid out like the lab and in the unchanged format. The runner refuses
  it before anything boots when it sits inside the clone or the reports
  directory, holds the reports directory or the lab key, has a link on the way
  to a directory a container mounts, is not a directory, is set and empty, or
  lacks a file a scenario names. Every report names the workspace, and its
  commit when it is the top of a git checkout.
- `examples/helpdesk-payouts/`: a worked example laid out as a workspace, the
  way a team deploying the enforcer would write it (its own principal, agent,
  policy, approver and security contract) against the lab's victims. Copy it
  out of the clone and run it with `LAB_WORKSPACE`; its README shows the
  commands and what a deliberate failure looks like. A test loads every
  directory under `examples/` as a workspace.
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
  on `evidence-net`, an internal network it shares with the enforcer alone)
  run the pinned OpenTelemetry Collector with only its file exporter
  (`append: true`, so a restart does not truncate what it already wrote), so
  the enforcer's OTLP export lands somewhere the runner can read it and
  nothing else on the lab's networks can reach. `runner/otlp.go` decodes that
  file with `internal/evidence.DecodeOTLP` and writes it out as the run's
  `evidence.jsonl`, one event per line, refusing an empty or eventless export
  rather than writing an empty one; a decode error never leaves a partial
  trail in its place.
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
  says. `compose/Dockerfile.approver` puts it beside the enforcer image's
  `/enforcer/control`.
- Documentation machinery: page frontmatter, budgets and `covers` in
  `docs/docs.json`, checked by `make docs-frontmatter`; a generated
  `docs/README.md`; `make docs-impact` for the pages a change makes suspect.
  The link check no longer reads untracked run reports.
- Scenarios the enforcer decides: a `gateway:` block names the scenario's part
  of the enforcer's configuration and its policy; the runner signs the policy
  for the run with the lab key (`make lab-key`, made once per machine outside
  the clone and the reports by the enforcer's own keygen), assembles the
  configuration around it from the keys a scenario may set (a dotted key is
  refused), boots the enforcer from its pinned image with the collector, reads
  the trail after the spool drained with nothing quarantined, truncated,
  refused or dropped and the collector flushed, and fails a run whose enforcer
  does not report the pinned commit or whose container does not run the pinned
  image. `gateway.upstream_tenants` puts a victim in a tenant of its own.
  `tool-02` is the first such scenario.
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
- Compose topology in `compose/`, every network marked internal so nothing in
  the lab has a route out.
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

### Changed

- The enforcer is pinned to its public release `v0.3.0-alpha` (commit
  `14153928d7cb0df18533856c2c6115b6693e92dc`, tree
  `7540e1f1a77892946d11c4734ffe26a9cc7ae6c0`), which anyone can fetch; the
  pin before the public releases was a commit the public repository does not
  serve. `versions.env` gains `ENFORCER_RELEASE`, and
  `scripts/fetch-enforcer.sh` refuses the commit when that release's tag at
  `ENFORCER_REPOSITORY` names another; the commit and its tree still decide
  what is built. The lab moved through `v0.2.0-alpha` (commit
  `471e18a0aec5e6201ea1a23c89ba0d1b926bbc33`, tree
  `0b3b35ccd5639ba50816c47f7d83c63e3fbebf59`), which brought the four changes
  below that name the new pin. `v0.3.0-alpha` brought none: its wire
  contract, the OTLP goldens and every tool fingerprint are those of
  `v0.2.0-alpha`, and the catalogue and the example grade the same,
  `mode-01` included.
- The enforcer exports its trail to the collector over TLS. At the new pin it
  sends plaintext only to a loopback address and refused to start with the
  lab's plaintext export. The runner makes a CA for each run, signs a
  certificate for `collector` with it and never writes the CA's key; the
  collector reads its certificate and key from the run's `collector-tls/`,
  and the enforcer trusts the CA from `export-ca/` beside the decision point
  double's. The collector's key is removed once the run's services are down.
- `evidence/run-id` asserts how the enforcer names runs at the new pin: every
  event on a request carries the one run it minted, except the events of a
  request whose proposal is tagged `flow.v1.state=uncomputed` (a call refused
  before it had a run, such as one to an unclassified tool), which carry
  none; no event names another run; and no envelope names a run of its own.
  The trail's run is the one its first proposal naming a run names; an event
  naming any other is not read as this run's and fails the check. The
  enforcer's run id is not the lab's, so the trail is tied to the lab's run
  by the directory the runner created, as before.
- The approver reads the `upstream` line `approvals list` prints at the new
  pin; a script cannot match on it yet.
- A named gap may be one the lab cannot configure yet as well as one the
  system lacks (`docs/lab-files.md`). `gaps-01` is now that kind: at the new
  pin the gateway keeps a run's flow, but it reads what a tool returns only
  from the operator's `returns` declaration, which the lab's classification
  cannot make yet, so the send stays `INDETERMINATE` where a `DENY` is
  wanted.
- `make docs-frontmatter` runs in `make quality`: every page under `docs/`
  carries frontmatter (`docs/lab-files.md` as a contract, exempt from a word
  budget), `docs/index.md` gave way to the generated `docs/README.md`, and the
  approver's, the decision point double's and two victims' READMEs fit the
  300-word folder budget.
- `tool-01`, `auth-01` and `flow-01` run against the enforcer instead of the
  stub, under new names: `tool-01-a-payout-change-annotated-read-only-is-denied-as-a-write`,
  `auth-01-an-injected-administrator-override-grants-no-export` and
  `flow-01-a-private-read-is-not-mailed-to-an-untrusted-sink`. Their declared
  verdicts in `config/scenarios/` are gone.
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
- `-scenario <path>` is read relative to the workspace (the clone by default)
  and refused outside it; a scenario found by identifier or by `-all` that
  links outside it is refused the same way. `-reports` inside a directory a
  container mounts is refused.
- `make check-hygiene` refuses a path that exists only on a maintainer's
  machine: a home directory other than the distroless images' own, the macOS
  temporary directories by their real paths, or a sibling checkout of either
  system under test. A URL segment that looks like one passes.
- The runner builds every image a scenario's profiles use, the agent's
  included, before it starts anything, and nothing after: the plane's policy
  goes stale on a clock that starts when it loads, and building the agent
  during the probes had spent that clock. A build that fails brings nothing up
  and fails every service's boot check with the build's error.
- A report's image line says the label matches the pin rather than the image,
  and for the enforcer adds whether the image's tree label is `ENFORCER_TREE`.
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
  service carries no `build:` and the runner's build before a run skips it,
  so the image it checked against `VERIFIER_VERSION` is the one that runs.

### Removed

- The stub gateway: `services/stub-gateway/`, `config/stub-gateway/`, its
  compose service and the `stub` profile. The enforcer decides every
  trajectory scenario; a scenario carrying `stub:` is refused at load as an
  unknown key, and one without `gateway:` or without the `enforcer` profile is
  refused. The runner no longer pairs a trail by a step number the trail
  carries.

### Fixed

- A run directory is the runner's at 0755 and each part a service writes into
  is sticky, and the runner writes every file of its own as a new one: another
  local user could replace a run's gateway part or plant a link the runner
  wrote its report through. A reports directory inside the clone is taken only
  at or under its `reports/`, where `attacks/`, which attacker-web serves, was
  accepted; the placement checks compare directories, so another spelling of
  the clone does not pass; an existing reports directory keeps its mode.
- Journals and the collector's export are read as regular files of a bounded
  size, never through a link, and opening them cannot block on a FIFO.
- `scripts/lab-key.sh` compares every directory above the key's place, with
  links resolved, with the clone by device and inode, also when the script is
  reached through a link; it refuses a `.` or `..` component and a path
  holding a control character, leaves an existing parent's mode alone, and
  mounts only a fresh directory of its own into the keygen container.
- victim-shell's `cat` reads regular files only, at most 1 MiB in all,
  where `cat /dev/zero` filled the container's memory and a pipe could hold it.

- The enforcer's agent listener binds the enforcer's own address on
  `agent-net`, where it bound every interface, so a container on
  `tool-net`, `evidence-net` or `pdp-net` could open a session as the
  configured principal. Each run gives `agent-net` a /27 of `10.231.0.0/16`,
  picked from the run id's random suffix, and the enforcer a fixed address
  in its upper half (`LAB_AGENT_SUBNET`, `LAB_AGENT_RANGE`,
  `LAB_ENFORCER_ADDRESS`, set by the runner); a lab brought up by hand
  uses the last /27, which no run is given.
- `make enforcer-image` builds again from a fresh clone: the tool lister a
  chaos scenario runs inside a victim is its own command, `relist`, and
  `compose/healthprobe`, which the enforcer image builds without the lab's
  module, is held to the standard library by a test.
- On a Linux host the runner can read what the lab's services write: every
  victim's journal, both doubles' journals and the scripted agent's log and
  trace are created mode 0644 whatever the umask, where they were 0600 and
  owned by the services' uid, so a runner under any other uid graded every
  effect as "no journal". Docker Desktop hid this by mapping every access to
  the invoking user. Directories keep their modes.

Nothing is released yet.
