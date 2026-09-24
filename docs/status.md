# Status

What exists, stated once, so no other page has to guess.

Labels: `implemented` runs and is tested · `experimental` runs, may change
without notice · `planned` does not exist.

| Component | Status | Notes |
|---|---|---|
| Repository rules and quality gate | implemented | `make quality` |
| Pinned versions of the systems under test | implemented | `versions.env`, every pulled image by tag and index digest; `make images` builds the enforcer from `git archive` of its commit, refused unless the archive hashes to that commit's tree, and the verifier from its hash-locked release; every report prints the pins, the machine, and each image on this machine with its ID and the pin its label names. The verifier image runs in the `verifier` profile and, as `trace-verifier`, in the `trace` profile; compose never pulls or builds it, and a verifier or trace run is refused unless the local image is labelled with `VERIFIER_VERSION`. The enforcer image runs in the `enforcer` profile; a run the enforcer decides fails unless its own enforcer container runs the image tagged with the pin and that image was built from it |
| Trajectory and scenario file formats | implemented | `internal/labspec`; an unknown key is refused and the two files cross-validate. Steps pair with the trails they open by order and by tool, can wait, retry a held call, resume a held trail or open none; named gaps are their own suite. A verifier scenario has probe steps instead of a trajectory. The trail cannot tell which retry resumed a hold. The catalogue grades every shape against the enforcer's trails at its pin: held, resumed, opening none, blocked and a named gap |
| Evidence, journal and assertion readers | implemented | `internal/evidence` mirrors the enforcer's v1 contract at its pinned commit and reads its OTLP log export; `internal/journal`, `internal/assertion`. `runner/otlp.go` decodes the collector's file export into a run's `evidence.jsonl`, refusing an empty or eventless export rather than writing an empty one. A run the enforcer decides reads its trail this way, after the enforcer's `/healthz` shows nothing unacknowledged and nothing quarantined, truncated, refused or dropped |
| Documentation checks | experimental | `make docs-check` (in the gate): local links over the files the repository lists, the index `docs/README.md` current, the frontmatter check and both scripts compiled. `make docs-frontmatter` (not in the gate yet; red until the pages are migrated) checks frontmatter, budgets and `covers` from `docs/docs.json`. `make docs-impact` names the pages a change makes suspect |
| Compose topology | implemented | `compose/`; each service mounts only the part of the run it writes (victims, the double and the approver `journals/`, the agent `agent/`); the victims and the chaos proxy (profile `chaos`) on `tool-net`; the stub (profile `stub`) or the enforcer (profile `enforcer`) as the only service on `agent-net`; the collector with the enforcer alone on `evidence-net`; the decision point double (profile `pdp`) with the enforcer alone on `pdp-net`; the approver (profile `approvals`) alone on `approver-net`; the trace verifier (profile `trace`) alone on `trace-net`; every network internal. Each run is its own compose project |
| Stub gateway | experimental | replays the declared verdicts in `config/scenarios/`; it decides nothing. It lists an upstream's tools once at startup as well as on demand, so a victim counting its own listings is one ahead of the agent |
| Victim tool servers | implemented | all six: crm, db, fs, shell, mail, web; each lies in the way its README states |
| AuthZEN decision point double | experimental | `services/pdp-double`: HTTPS with a CA it makes in memory and scopes to its own names, answers scripted per scenario (allow, deny, obligation, timeout, 500, no echo, malformed, extra member; unscripted is denied), every question journalled. `pdp-01`..`03` use it and grade its journal |
| Approver | experimental | `services/approver`: answers held approvals per scenario script through the enforcer's own `approvals` command, never while no plane holds the directory; every action and every outcome it cannot confirm journalled. `approval-01`..`05` and `trace-01` use it, and the approval scenarios grade its journal |
| Enforcer in the lab | experimental | `make lab-key` once, then a scenario with `gateway:` runs the enforcer built from its pinned commit: its policy signed for the run with the lab key by the enforcer's own `policy sign`, its configuration assembled from the scenario's part (only the keys the lab lets a scenario set) and the lab's classification pinned to the fingerprints its `doctor` printed and to the listing snapshots they were taken from (`make classify-victims`), its trail read from the collector after the spool drained; the run fails when the enforcer does not report the pinned commit |
| Scripted agent | implemented | `agents/scripted`; replays a trajectory and forwards each step's output into the next; with `-trace` it writes its own record of the run in the verifier's native trace dialect, with each call's effect and approval |
| Scenario runner and assertions | implemented | `make scenario ID=...`; boot, topology, replay, decisions, trails, effects (every journal line of the run, served or refused, named by the scenario) and evidence checks; a verifier scenario is graded on boot, the verifier's reach and routing tables, each step's exit code, pin and JSON report, and every victim's journal; a run the enforcer decides also on the mode every event records and the executed digest on every completion; a scenario with `trace:` is also graded on the verifier's analysis of the agent's trace against a contract; every run in a compose project of its own |
| Scenario catalogue | experimental | three scenarios against the stub (one `ALLOW`, one `DENY`, one `INDETERMINATE`); 27 against the enforcer at its pinned commit: reads (`tool-02`), rules and a stale policy (`rule-01`..`03`), an unclassified tool (`tool-03`), tenancy (`tenant-01`, `tenant-03`), approvals (`approval-01`..`05`), the decision point (`pdp-01`..`03`), obligations (`obligation-01`), modes (`mode-01`, `mode-02`), a full spool (`evidence-01`, `evidence-02`), one named gap (`gaps-01`), two whose trace the verifier grades (`scenarios/trace/`) and four under chaos (`chaos-01`..`04`); four verifier scenarios against the verifier at 0.26.1. Two are red by design (below): `verify-04` and `mode-01` |
| Attack payload catalogue | experimental | four indirect injections in `attacks/`, served by `attacker-web` |
| Chaos matrix | experimental | a scenario's `chaos:` faults, applied after boot and lifted before the drain, each graded from a record that shows it in place (`docs/lab-files.md`): a latency or a hang on one victim's answers through the proxy (profile `chaos`), the collector down, a second listing of a victim's tools. `chaos-01`..`04` against the enforcer at its pinned commit: a latency inside `upstream.call_timeout` runs, a hang closes `ACTION_FAILED` with `RESULT_STATUS_TIMEOUT` at that bound after the victim served the call, a collector outage is waited out and drained whole, and `fs.read` described anew by victim-fs under the running gateway is blocked `ACTION_UNCLASSIFIED`. Planned: cut links, a full disk, clock skew, faults on the decision point's or the collector's own path |
| Verifier probes | experimental | `scenarios/verify/`: a pin written then compared, drift on victim-fs, no drift on a stable manifest, no pin reported unverified, drift at its catalogued severity (red). Expectations are copied from the verifier's own docs at 0.26.1. Each step, and each reach dial, starts a container from the image `make verifier-image` built |
| Verifier loop | experimental | a scenario with `trace:` has the pinned verifier grade the agent's own trace of a run through the enforcer against a security contract in `config/contracts/`; `trace-01` (approved change, a denied shell command, contract holds, exit 0) and `trace-02` (a widened policy lets the change run unapproved, exit 1 with the contract's HIGH finding) pass at the pins. The trace is the agent's record: the verifier's verdict cannot tell a change approved and run from one that never ran, which the decisions and effects checks do. Comparing two runs' reports (`verifier diff`) and probing the gateway's own listener are planned |
| Live model overlay | planned | recorded to cassettes, replayed in CI |
| Benchmarks | planned | decision latency against policy bundle size |

The three stub scenarios run end to end and pass, and each was made to fail on
purpose by changing what the stub replays. Their green says the runner asserts
what it claims; it says nothing about the enforcement plane, because the
verdicts come from `config/scenarios/<id>.yaml`, written by the scenario's
author.

`tool-02-permitted-read-is-recorded-by-the-enforcer` is the first scenario the
enforcer itself decides: two reads its policy allows, graded from the trail the
enforcer exported and the victims' journals. It was made to fail on purpose by
signing a policy that allows only writes: the enforcer denied both reads with
`NO_MATCHING_RULE` and the victims served nothing. Its green is evidence about
that one path at the pinned commit and nothing wider.

The catalogue against the enforcer states each expectation from the
enforcer's docs at its pin, cited page and line in each scenario, and every
scenario was made to fail on purpose once by changing the policy, the
configuration, a double's script or a step, never the expectation.
`mode-01-observe-runs-what-it-records` is red by design, on a contradiction
recorded as a finding for the enforcer: under `OBSERVE` a call whose decision
refused its action digest completes with an executed digest, as if the bytes
sent were authorized. While it is red, a second regression in it goes unseen.
Whether the decision point was asked is graded from its journal, since the
decision names it for a question it could not send as well. Not asserted:
the cause of `evidence-01`'s block and `evidence-02`'s unrecorded read, which
only the enforcer's `/healthz` shows. A
principal with no tenant cannot be configured at the pin (the gateway fills in
its own), so only the other one-sided tenant case is graded (`tenant-03`).
`plane/image` requires the image the run's enforcer container runs to carry
the `io.guardana.playground.enforcer.tree` label equal to `ENFORCER_TREE`.
Only `make enforcer-image` sets it, after checking that the archive hashes to
the commit's tree and that tree is the pinned one, so an image built by hand
with the pinned build arguments fails the check. The label is still a claim
the build makes about itself: an image that sets it by hand, or builds `FROM`
an image that carries it, passes. The lab binds the enforcer's agent listener on all
interfaces (`runner/gateway/config.go`), and the enforcer sits on four
networks, so a container on `tool-net` could open a session as the configured
principal; a call it made would fail `trails/opened` or the effects checks,
not pass them.

The chaos scenarios were each made to fail on purpose by changing a fault, the
configuration or the runner: a call timeout under the latency (`chaos-01`
closed the call `ACTION_FAILED` after 1.0s and the chaos check found it faster
than the latency), an obligation that shortens the held call's timeout to one
second (`chaos-02` closed it `RESULT_STATUS_TIMEOUT` after 1.0s, outside the
window its configuration sets, and the chaos check went red on the duration),
a call timeout of three minutes, past the agent's two-minute bound on a replay
(`chaos-02` closed the call `RESULT_STATUS_FAILURE`, and the chaos check went
red on the status), a collector never started again (`chaos-03`'s fault was not
lifted and the trail never drained), and no second listing (`chaos-04` allowed
and ran `fs.read`). What they do not show: the answer the agent gets
for a call cut off at the timeout, which the enforcer's docs do not state; the
enforcer's behaviour when a victim's connection is cut rather than held; and a
drift the enforcer learns of any other way than the victim announcing it.
`list.ttl` is the agent's cache of the gateway's own list and never makes the
gateway read an upstream again, so a victim that changed its tools without
announcing it stays classified as first listed.

The verifier scenarios run the verifier's `probe` at its pin against the
victims. Three pass; each was made to fail on purpose by changing the steps, not
the expectations (a pin taken after the drift, a drifting server where a stable
one was expected, a pin where none was expected). Their green is evidence about
the verifier's MCP manifest checks at 0.26.1 and nothing wider.
`verify-04-drift-severity-is-the-catalogued-one` is red, and stays red until the
verifier or its rule catalogue changes: the catalogue lists
`guardana.agent.mcp_server_manifest` as `HIGH` and the verifier reports the drift
`CRITICAL`, so `make scenarios` is red on it.

The two trace scenarios run the same calls under two policies. Each was made to
fail on purpose: `trace-02` with a contract that no longer covers the payout
change turned only its two trace checks red (the verifier exited 0 with no
finding), `trace-01` under the widened policy turned its decisions, its
trails, its effects and its trace checks red, and `trace-01` under a policy that
also allows its shell command turned the command's decision, the shell's
effects and two trace checks red, the verifier reporting `never-shell` as
`CRITICAL`. Their green is evidence about the verifier's
`approval_required` and `forbidden_sink` assertions over a trace this lab's
agent writes, at 0.26.1, and nothing wider.

Nothing has been measured yet. Any number this repository publishes later comes
with the machine that produced it.
