# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Changed

- `make bootstrap` installs every gate tool into `./bin` from its pinned,
  hashed release asset on Linux (glibc) and macOS, amd64 and arm64. It
  downloads and verifies each asset on every run, keeps a binary already in
  `./bin` only when its bytes are the verified one's, and records their
  sha256. `make quality` runs the tools from `./bin` by path, after
  `check-tools` has matched each with that record; Homebrew, or any tool
  elsewhere on `PATH`, is no longer used, and actionlint no longer lints
  workflow shell with whatever shellcheck a machine has.
- `make quality` and `make quality-quick` refuse a Go other than the version
  `go.mod` names (`scripts/check-go-version.sh`), and say to set `GOTOOLCHAIN`.
- Dependabot proposes a new version of a module or an action seven days after
  its release at the earliest.
- A run whose compose file does not load for its profile fails as
  `compose/loaded`, carrying compose's own message, before anything is built;
  it was an `indeterminate` boot with the reason on the console only. The
  quickstart names Compose 2.20 as the minimum (`depends_on.required`).
- Each service that writes a journal sees only its own directory,
  `journals/<server>/<server>.jsonl`, so a victim or the approver (which runs
  the enforcer's `control`) can no longer rewrite another's record. A journal
  directory holding anything else is not read: its effects fail as `journal
  unreadable`, naming what was there, as any journal that is there and cannot
  be read now does, apart from `no journal`. `make up` refuses a link on the
  way to a directory it makes, and `check-hygiene.sh` a link anywhere in the
  tree. The file format is unchanged.

### Fixed

- `check-actions-pinned.sh` reads every workflow, `.yaml` included, and every
  action kept under `.github/actions/`, and fails when it finds none: a
  `.yaml` workflow on a tag passed. `actionlint` reads the same workflows, and
  `zizmor` all of `.github`, `dependabot.yml` included.
- Each head pushed to `main` keeps its own run, neither cancelled nor held
  pending by the next push's, and a manual run of the scenario job no longer
  cancels a push's; a newer push to a pull request still supersedes its run.
  `quality` stops after 30 minutes.
- A `REPORTS` path holding a space reaches the runner whole from every make
  target, and the runner refuses an argument that is not a flag, where the
  tail of such a path was dropped. `.gitattributes` keeps every checkout at LF,
  so a script checked out on Windows still runs.

## 0.1.0 - 2026-10-03

### Added

- `obligation-02` grades `cap_amount` on what victim-pay committed: a charge
  of 50000 lands at 5000, one of 3000 at 3000, and a fractional amount the
  cap cannot apply to is blocked by the gateway. `flow-04` fetches a ticket
  (`attacks/pi-05`) asking for a payout to an account it names: a payout to
  the customer's own account before the ticket runs, the refund to the card
  after it runs, and both payouts after it, the customer's own account
  included, are denied as a toxic flow. `gaps-03` names a gap: a charge retried after
  its answer was lost charges again, with or without an idempotency key.
- `victim-pay`, a payments ledger: `pay.charge`, `pay.refund`, `pay.payout`
  and `pay.read_charge`, the first tools the lab classifies `TRANSACT`. Its
  lie is in `pay.charge`'s schema: a repeat with the same `idempotency_key`
  is said to return the first charge, and the server charges again. It
  enforces no amount cap, no tenancy and no de-duplication, so a cap or a
  flow is the enforcer's to decide. The three tools that move money record
  what they moved as the journal line's `effect`. A payout is classified
  `UNTRUSTED_EXTERNAL` whatever account it names.
- `expect.effects.<victim>.committed` states what each served call changed,
  in order, member for member, graded from the journal's `effect` as
  `effects/<victim>/committed`. Exhaustive like the counts: an effect the
  scenario does not state fails. Additive; the format stays
  `schema_version: 1`.
- A journal line may carry `effect`: what a served call changed, as a flat
  object of strings, booleans and integers within ±(2^53−1), written by the
  victim before the change is made (`victims/mcpserve` `Committing`). The
  writer and the reader refuse a value they cannot hold exactly. Additive: a
  journal without it reads as before.
- `docs/how-it-works/scenario-run.md` and `docs/how-it-works/verifier-run.md`:
  which service sits on which network, how a call is decided, held and graded,
  which record each check reads, and what a run leaves on disk, with the
  diagrams drawn from the files they name. `docs/reference/glossary.md` gives
  one name per concept.

- The lab's classification declares what each tool returns (`returns.trust`,
  `returns.sensitivity`, which the enforcer reads into a run's flow state) and
  where it sends (`trust_zone`), per tool: every tool but `shell.exec`, whose
  `cat` can read anything in its container. `flow-02` grades the enforcer's own
  example: a mail after an untrusted page is undetermined, and after a private
  read the same address is denied as a toxic flow. `flow-03` is `flow-01`'s
  positive control: the private read written to a sink classified trusted runs
  under `deny_external_sink` and a toxic-flow rule. `gaps-02` names what one
  undeclared result does to the run's reading.
- A step can state `result: { status: ... }`, graded on its trail's closing
  record, and `proposed_tags_include`, graded on the run-context tags of its
  proposal, where the enforcer records the flow state it decided by. Both are
  additive; the format stays `schema_version: 1`. `flow-01` grades its
  refused send's `BLOCKED` result.
- The runner refuses a trajectory whose principal, tenant, agent or
  environment is not the one its gateway part's listener names.

- A development mode for the enforcer: `make dev-scenarios
  CONTROL=<checkout> [ID=<scenario>]` builds it from a checkout's working
  tree, uncommitted changes included, as `playground-enforcer-dev:<tree12>`,
  the first 12 hex digits of the tree it built (`scripts/build-enforcer-dev.sh`),
  and runs the catalogue or one scenario
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
  (from a checkout to one green scenario, reading the report, what a red
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
- Victim tool servers — crm, db, fs, shell, mail and web — and the
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
- Attack payload catalogue in `attacks/`: indirect prompt injections served by
  `attacker-web`.

### Changed

- A trajectory step's `server` that is not one of the lab's victims is
  refused when the file loads, rather than failing the run. One list
  (`internal/labspec` `Victims`) is the one the runner fronts, and a test holds
  compose, the chaos proxies, `scripts/classify-victims.sh`, the listing
  snapshots, the classification and the fingerprints to it; the scripted
  agent's trace refuses a server it has no sink for instead of calling it
  `other`.
- `docs/status.md` is the inventory alone: one row per component with its
  label, what a green result does and does not show, and the page that
  explains it. The mechanism it described is on the how-it-works pages, and
  the change that turns a scenario red, where one is recorded, is in that
  scenario's header comment.
- The documentation check reads the heading a link names, inline or as a
  reference, every Mermaid fence and its diagram type, holds the how-it-works
  pages to the status labels, and refuses a scenario count stated outside
  `docs/status.md`, missing from it, or different from the scenario files.

- Red by design names the check each listed scenario fails on
  (`<id> <check>[,<check>...] <finding>`), and the judge passes the catalogue
  only when each listed scenario fails on exactly those checks and passes every
  other; a listed scenario that failed to load, boot or sign, or holds an
  indeterminate result, is no longer counted as red as listed.
- `make bootstrap` installs a release binary only when its sha256 is the one
  `scripts/tool-versions.env` pins for the platform, on Linux amd64 and arm64,
  and removes a download it refused; it checks each tool's version exactly, and
  the Makefile finds what it put in `./bin`; `buf` and `syft`, which nothing
  ran, are no longer installed.
- `gaps-01` is gone: with `returns` declared, the send it named is denied, and
  `flow-02` grades it.

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
  system lacks (`docs/lab-files.md`).
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

### Fixed

- A run directory is the runner's at 0755 and each part a service writes into
  is sticky, and the runner writes every file of its own as a new one: another
  local user could replace a run's gateway part or plant a link the runner
  wrote its report through. A reports directory inside the clone is taken only
  at or under its `reports/`, where `attacks/`, which attacker-web serves, was
  accepted; the placement checks compare directories, so another spelling of
  the clone does not pass; an existing reports directory keeps its mode.
- Journals, traces, the collector's export and the verifier's report are read
  as regular files of a bounded size, never through a link; opening one cannot
  block on a FIFO, and a FIFO swapped in after the check is refused, also where
  the file system gives it the removed file's inode number.
- `scripts/lab-key.sh` compares every directory above the key's place, with
  links resolved, with the clone by device and inode, also when the script is
  reached through a link; it refuses a `.` or `..` component and a path
  holding a control character, leaves an existing parent's mode alone, and
  mounts only a fresh directory of its own into the keygen container.
- `content_captured: false` fails on result text as well as argument text;
  every `ACTION_COMPLETED` has to carry a successful result; a victim the
  profile booted and the scenario does not name is held to having served
  nothing; a verifier scenario is refused a `gateway`, whose `expect.health`
  nothing read; `-all` refuses a file in a suite directory it would not run,
  a hidden directory there, a YAML file in any letter case directly under
  `scenarios/` and an entry there that does not resolve; a trail whose last
  line lacks its newline is refused.
- `check-hygiene.sh` refuses a hidden name anywhere in a path, where it looked
  at the first component only. The file list every guard reads takes each name
  as git stores it, where a name git quotes, such as one with a byte outside
  ASCII, was left out; a name holding a line break fails the list, and so does
  a nested repository or a submodule, whose files git does not list; outside
  git a link is listed like a file. The guards hand grep every name after
  `--`, where a file named like an option, such as `-q.md`, switched their
  content scans off; they read every file as text, where grep skipped one
  starting with a NUL byte and, under GNU grep in a UTF-8 locale, held back a
  line that is not UTF-8, and they refuse a file they cannot read, whose error
  they hid. `make fmt` and `fmt-check` stop when the list fails, where
  `fmt-check` read gofmt's standard input and said clean, set their own shell
  flags, which GNU Make 3.81 does not take from `.SHELLFLAGS`, and hand gofmt
  each name after `--`.
- victim-shell's `cat` reads regular files only, at most 1 MiB in all,
  where `cat /dev/zero` filled the container's memory and a pipe could hold it.
- victim-web refuses a redirect to another host than attacker-web, as it
  refuses a URL naming one, so a page it returns is attacker-web's, and stops
  after ten redirects.

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
