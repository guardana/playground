---
title: Status
summary: What exists in the lab today, component by component, with what each green result does and does not show.
type: project
audience: [engineering, product]
covers: [agents/**, attacks/**, compose/**, config/**, examples/**, internal/**, runner/**, scenarios/**, services/**, trajectories/**, victims/**, versions.env, Makefile]
---

# Status

What exists, stated once, so no other page has to guess.

Labels: `implemented` runs and is tested · `experimental` runs, may change
without notice · `planned` does not exist.

| Component | Status | Notes |
|---|---|---|
| Repository rules and quality gate | implemented | `make quality`; `check-hygiene.sh` also refuses a path only a maintainer's machine has: a home directory other than the distroless images' own, the macOS `/tmp` by its real path and the macOS per-user temporary directory, a sibling checkout of either system under test; a path counts only where one starts, so a URL segment passes |
| Pinned versions of the systems under test | implemented | `versions.env`, every pulled image by tag and index digest; `make images` builds the enforcer from `git archive` of its commit, refused unless the archive hashes to that commit's tree, and the verifier from its hash-locked release; every report prints the pins, the machine, and each image on this machine with its ID, the pin its label names and whether that label matches, and for the enforcer whether its tree label is `ENFORCER_TREE`. The verifier image runs in the `verifier` profile and, as `trace-verifier`, in the `trace` profile; compose never pulls or builds it, and a verifier or trace run is refused unless the local image is labelled with `VERIFIER_VERSION`. The enforcer image runs in the `enforcer` profile; a run the enforcer decides fails unless its own enforcer container runs the image tagged with the pin and that image was built from it |
| Trajectory and scenario file formats | implemented | `internal/labspec`; an unknown key is refused and the two files cross-validate. Steps pair with the trails they open by order and by tool, can wait, retry a held call, resume a held trail or open none; named gaps are their own suite. A verifier scenario has probe steps instead of a trajectory. The trail cannot tell which retry resumed a hold. The catalogue grades every shape against the enforcer's trails at its pin: held, resumed, opening none, blocked and a named gap |
| Evidence, journal and assertion readers | implemented | `internal/evidence` mirrors the enforcer's v1 contract at its pinned commit and reads its OTLP log export; `internal/journal`, `internal/assertion`. `runner/otlp.go` decodes the collector's file export into a run's `evidence.jsonl`, refusing an empty or eventless export rather than writing an empty one. A run the enforcer decides reads its trail this way, after the enforcer's `/healthz` shows nothing unacknowledged and nothing quarantined, truncated, refused or dropped |
| Documentation checks | experimental | `make docs-check` (in the gate): local links over the files the repository lists, the index `docs/README.md` current, the frontmatter check and both scripts compiled. `make docs-frontmatter` (in the gate) checks every page's frontmatter, every README's word budget and `covers` from `docs/docs.json`. `make docs-impact` names the pages a change makes suspect |
| Compose topology | implemented | `compose/`; each service mounts only the part of the run it writes (victims, the double and the approver `journals/`, the agent `agent/`); the victims and the chaos proxy (profile `chaos`) on `tool-net`; the enforcer (profile `enforcer`) as the only service on `agent-net` beside the agent, at a fixed address in a /27 of `10.231.0.0/16` the runner picks per run from the run id's random suffix, the only address its agent listener binds (a subnet already in use fails the boot); the collector with the enforcer alone on `evidence-net`; the decision point double (profile `pdp`) with the enforcer alone on `pdp-net`; the approver (profile `approvals`) alone on `approver-net`; the trace verifier (profile `trace`) alone on `trace-net`; every network internal. Each run is its own compose project; the trajectories, contracts and the doubles' scripts are bind-mounted read-only from `LAB_WORKSPACE`, which the runner sets to the clone or to the workspace it was given, and a missing source directory is refused rather than created |
| Victim tool servers | implemented | all six: crm, db, fs, shell, mail, web; each lies in the way its README states |
| AuthZEN decision point double | experimental | `services/pdp-double`: HTTPS with a CA it makes in memory and scopes to its own names, answers scripted per scenario (allow, deny, obligation, timeout, 500, no echo, malformed, extra member; unscripted is denied), every question journalled. `pdp-01`..`03` use it and grade its journal |
| Approver | experimental | `services/approver`: answers held approvals per scenario script through the enforcer's own `approvals` command, never while no plane holds the directory; every action and every outcome it cannot confirm journalled. `approval-01`..`05` and `trace-01` use it, and the approval scenarios grade its journal |
| Enforcer in the lab | experimental | `make lab-key` once, then a scenario with `gateway:` runs the enforcer built from its pinned commit: its policy signed for the run with the lab key by the enforcer's own `policy sign`, its configuration assembled from the scenario's part (only the keys the lab lets a scenario set) and the lab's classification pinned to the fingerprints its `doctor` printed and to the listing snapshots they were taken from (`make classify-victims`), its trail read from the collector after the spool drained; the run fails when the enforcer does not report the pinned commit |
| Scripted agent | implemented | `agents/scripted`; replays a trajectory and forwards each step's output into the next; with `-trace` it writes its own record of the run in the verifier's native trace dialect, with each call's effect and approval |
| Scenario runner and assertions | implemented | `make scenario ID=...`; boot, topology (the agent reaches the enforcer and nothing else; from the victim the trajectory calls first, and the decision point double when its profile is up, the enforcer refuses a connection at its address on their network and its agent-net address is out of reach; the enforcer's own output names that address as the only one its agent listener bound), replay, decisions, trails, effects (every journal line of the run, served or refused, named by the scenario) and evidence checks; a verifier scenario is graded on boot, the verifier's reach and routing tables, each step's exit code, pin and JSON report, and every victim's journal; a run the enforcer decides also on the mode every event records and the executed digest on every completion; a scenario with `trace:` is also graded on the verifier's analysis of the agent's trace against a contract; every run in a compose project of its own; `LAB_WORKSPACE=<dir>` runs scenarios from a directory outside the clone, laid out like the lab (`docs/lab-files.md`), refused before boot inside the clone or the reports, with the reports inside it, with a mounted directory that is a link, with the lab key inside it, or when a named file is missing, and named with its commit in every report when it is the top of a checkout; every image the profile builds, the agent's included, is built before anything starts; every file a service writes and the runner reads is created readable by another uid, so the runner works on a Linux host as an ordinary user (the catalogue on a Linux Docker host as uid 1000: 32 of 34 pass; the two that fail are `mode-01` and `verify-04`, red by design) |
| Scenario catalogue | experimental | 30 against the enforcer at its pinned commit: reads (`tool-02`), a payout change its server annotates read-only decided as a write (`tool-01`), rules and a stale policy (`rule-01`..`03`), an unclassified tool (`tool-03`), tenancy (`tenant-01`, `tenant-03`), an export granted to another principal refused to the configured one after a page claims an administrator's authority (`auth-01`), a send after a private read refused under `deny_external_sink` at a sink classified untrusted (`flow-01`), approvals (`approval-01`..`05`), the decision point (`pdp-01`..`03`), obligations (`obligation-01`), modes (`mode-01`, `mode-02`), a full spool (`evidence-01`, `evidence-02`), one named gap (`gaps-01`), two whose trace the verifier grades (`scenarios/trace/`) and four under chaos (`chaos-01`..`04`); four verifier scenarios against the verifier at 0.26.1. Two are red by design (below): `verify-04` and `mode-01` |
| Attack payload catalogue | experimental | four indirect injections in `attacks/`, served by `attacker-web` |
| Chaos matrix | experimental | a scenario's `chaos:` faults, applied after boot and lifted before the drain, each graded from a record that shows it in place (`docs/lab-files.md`): a latency or a hang on one victim's answers through the proxy (profile `chaos`), the collector down, a second listing of a victim's tools. `chaos-01`..`04` against the enforcer at its pinned commit: a latency inside `upstream.call_timeout` runs, a hang closes `ACTION_FAILED` with `RESULT_STATUS_TIMEOUT` at that bound after the victim served the call, a collector outage is waited out and drained whole, and `fs.read` described anew by victim-fs under the running gateway is blocked `ACTION_UNCLASSIFIED`. Planned: cut links, a full disk, clock skew, faults on the decision point's or the collector's own path |
| Verifier probes | experimental | `scenarios/verify/`: a pin written then compared, drift on victim-fs, no drift on a stable manifest, no pin reported unverified, drift at its catalogued severity (red). Expectations are copied from the verifier's own docs at 0.26.1. Each step, and each reach dial, starts a container from the image `make verifier-image` built |
| Verifier loop | experimental | a scenario with `trace:` has the pinned verifier grade the agent's own trace of a run through the enforcer against a security contract in `config/contracts/`; `trace-01` (approved change, a denied shell command, contract holds, exit 0) and `trace-02` (a widened policy lets the change run unapproved, exit 1 with the contract's HIGH finding) pass at the pins. The trace is the agent's record: the verifier's verdict cannot tell a change approved and run from one that never ran, which the decisions and effects checks do. Comparing two runs' reports (`verifier diff`) and probing the gateway's own listener are planned |
| Worked example | experimental | `examples/helpdesk-payouts/`: one adopter scenario as a workspace (policy, gateway part, approver script, contract, trajectory, scenario) for a helpdesk assistant that reads customers, changes a payout account only after an approval and mails nothing outside; run by copying it out of the clone and setting `LAB_WORKSPACE`; `internal/labcheck` loads every `examples/*/` as a workspace (each scenario loads, matches its trajectory and is decided by the enforcer, every named file exists, no file is unused or outside the directory the runner reads it from, no file is a byte copy of a lab file); green from a copy at the pinned enforcer and verifier, red when its policy lets the payout change run without an approval, and red when it lets the mail out. `make scenarios` does not run it |
| Continuous integration | experimental | `.github/workflows/ci.yml` runs `make quality`. `.github/workflows/scenarios.yml` fetches the enforcer's commit from `ENFORCER_REPOSITORY` by its id, anonymously (`scripts/fetch-enforcer.sh`, red with the reason when it cannot), then runs `scripts/ci-scenarios.sh`: both images from their pins, a lab key of its own, the whole catalogue judged by `runner -all -red-by-design scenarios/red-by-design.txt` (green only when the scenarios not passing are exactly the listed ones, each red on a record), and every `examples/*/` from a copy outside the clone. It has not run on GitHub: the repository has no remote yet, and `https://github.com/guardana/control` serves no ref for `ENFORCER_COMMIT`, so the fetch is red there today |
| Live model overlay | planned | recorded to cassettes, replayed in CI |
| Benchmarks | planned | decision latency against policy bundle size |

`tool-01`, `auth-01` and `flow-01` were written for a stub gateway, since
removed, and are now decided by the enforcer, each asserting what no other
scenario does: the operator's classification, not a tool's `readOnlyHint`,
decides the effect class; an export the policy grants only to another
principal is refused to the listener's configured one (the page claiming an
administrator's authority puts nothing into the call, so this shows the rule's
principal scoping, not the gateway resisting the page); and a send after a
private read is refused under the MCP adapter's `deny_external_sink`, at a sink
the lab classifies untrusted. No lab tool is classified trusted, so a trusted
sink passing under that obligation is not shown.
The adapter closes that refusal `ACTION_FAILED` with the code
`OBLIGATION_NOT_UNDERSTOOD`, the same code as an obligation it cannot apply,
so the trail alone does not say which of the two stopped the send; the lab
does not grade that code.

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
decision names it for a question it could not send as well. The cause of
`evidence-01`'s blocks and `evidence-02`'s unrecorded read, which no trail
records, is graded from the enforcer's `/healthz` after the replay
(`expect.health`: blocks by reason code, `reads_unrecorded`,
`sink_failures_before_effect`), kept in the run directory as `healthz.json`. A
principal with no tenant cannot be configured at the pin (the gateway fills in
its own), so only the other one-sided tenant case is graded (`tenant-03`).
`plane/image` requires the image the run's enforcer container runs to carry
the `io.guardana.playground.enforcer.tree` label equal to `ENFORCER_TREE`.
Only `make enforcer-image` sets it, after checking that the archive hashes to
the commit's tree and that tree is the pinned one, so an image built by hand
with the pinned build arguments fails the check. The label is still a claim
the build makes about itself: an image that sets it by hand, or builds `FROM`
an image that carries it, passes. The enforcer's agent listener binds the
enforcer's own address on `agent-net` and nothing else, although the enforcer
sits on four networks. Three checks read it: `listener-closed-to/<service>`
has a service on another of those networks list the enforcer's tools at its
service name, which there resolves to the enforcer's address on that network,
and passes only on a refused connection at that address;
`agent-address-unreachable-from/<service>` has it list them at the agent-net
address, and passes only on no route or no answer; and
`listener-bound-to-agent-net` reads the enforcer's own output, which names the
address its agent listener bound, and passes only when that is the agent-net
address alone. Any other error is indeterminate. The probes run from the first
victim the trajectory calls and from the decision point double, not from the
collector or the chaos proxy, whose images carry no lister; the enforcer's
own line covers every network. Two runs at once share a subnet one time in
2047; docker refuses the second network, and that run's boot fails with the
reason in its boot record.

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
