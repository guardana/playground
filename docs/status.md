# Status

What exists, stated once, so no other page has to guess.

Labels: `implemented` runs and is tested · `experimental` runs, may change
without notice · `planned` does not exist.

| Component | Status | Notes |
|---|---|---|
| Repository rules and quality gate | implemented | `make quality` |
| Pinned versions of the systems under test | implemented | `versions.env`, every pulled image by tag and index digest; `make images` builds the enforcer from `git archive` of its commit, refused unless the archive hashes to that commit's tree, and the verifier from its hash-locked release; every report prints the pins, the machine, and each image on this machine with its ID and the pin its label names. The verifier image runs in the `verifier` profile and, as `trace-verifier`, in the `trace` profile; compose never pulls it, and a verifier or trace run is refused unless the local image is labelled with `VERIFIER_VERSION`. The enforcer image runs in the `enforcer` profile; a run the enforcer decides fails unless its own enforcer container runs the image tagged with the pin and that image was built from it |
| Trajectory and scenario file formats | implemented | `internal/labspec`; an unknown key is refused and the two files cross-validate. Steps pair with the trails they open by order and by tool, can wait, retry a held call, resume a held trail or open none; named gaps are their own suite. A verifier scenario has probe steps instead of a trajectory. The trail cannot tell which retry resumed a hold. `tool-02` and `trace-01` have graded real enforcer trails, `trace-01` a held call resumed by its retry; the gap shape has not met one yet |
| Evidence, journal and assertion readers | implemented | `internal/evidence` mirrors the enforcer's v1 contract at its pinned commit and reads its OTLP log export; `internal/journal`, `internal/assertion`. `runner/otlp.go` decodes the collector's file export into a run's `evidence.jsonl`, refusing an empty or eventless export rather than writing an empty one. A run the enforcer decides reads its trail this way, after the enforcer's `/healthz` shows nothing unacknowledged and nothing quarantined, truncated, refused or dropped |
| Documentation checks | experimental | `make docs-check` (in the gate): local links over the files the repository lists, the index `docs/README.md` current, the frontmatter check and both scripts compiled. `make docs-frontmatter` (not in the gate yet; red until the pages are migrated) checks frontmatter, budgets and `covers` from `docs/docs.json`. `make docs-impact` names the pages a change makes suspect |
| Compose topology | implemented | `compose/`; each service mounts only the part of the run it writes (victims, the double and the approver `journals/`, the agent `agent/`); the victims on `tool-net`; the stub (profile `stub`) or the enforcer (profile `enforcer`) as the only service on `agent-net`; the collector with the enforcer alone on `evidence-net`; the decision point double (profile `pdp`) with the enforcer alone on `pdp-net`; the approver (profile `approvals`) alone on `approver-net`; the trace verifier (profile `trace`) alone on `trace-net`; every network internal. Each run is its own compose project |
| Stub gateway | experimental | replays the declared verdicts in `config/scenarios/`; it decides nothing. It lists an upstream's tools once at startup as well as on demand, so a victim counting its own listings is one ahead of the agent |
| Victim tool servers | implemented | all six: crm, db, fs, shell, mail, web; each lies in the way its README states |
| AuthZEN decision point double | experimental | `services/pdp-double`: HTTPS with a CA it makes in memory and scopes to its own names, answers scripted per scenario (allow, deny, obligation, timeout, 500, no echo, malformed, extra member; unscripted is denied), every question journalled. No scenario uses it yet |
| Approver | experimental | `services/approver`: answers held approvals per scenario script through the enforcer's own `approvals` command, never while no plane holds the directory; every action and every outcome it cannot confirm journalled. `trace-01` uses it |
| Enforcer in the lab | experimental | `make lab-key` once, then a scenario with `gateway:` runs the enforcer built from its pinned commit: its policy signed for the run with the lab key by the enforcer's own `policy sign`, its configuration assembled from the scenario's part (only the keys the lab lets a scenario set) and the lab's classification pinned to the fingerprints its `doctor` printed and to the listing snapshots they were taken from (`make classify-victims`), its trail read from the collector after the spool drained; the run fails when the enforcer does not report the pinned commit |
| Scripted agent | implemented | `agents/scripted`; replays a trajectory and forwards each step's output into the next; with `-trace` it writes its own record of the run in the verifier's native trace dialect, with each call's effect and approval |
| Scenario runner and assertions | implemented | `make scenario ID=...`; boot, topology, replay, decisions, trails, effects and evidence checks; a verifier scenario is graded on boot, the verifier's reach and routing tables, each step's exit code, pin and JSON report, and every victim's journal; a scenario with `trace:` is also graded on the verifier's analysis of the agent's trace against a contract; every run in a compose project of its own |
| Scenario catalogue | experimental | three scenarios against the stub (one `ALLOW`, one `DENY`, one `INDETERMINATE`); one against the enforcer at its pinned commit (`tool-02`, reads allowed); two through the enforcer whose trace the verifier grades (`scenarios/trace/`); four verifier scenarios against the verifier at 0.26.1, one of them red (below) |
| Attack payload catalogue | experimental | four indirect injections in `attacks/`, served by `attacker-web` |
| Chaos matrix | planned | latency, timeouts, cut links, full disk, clock skew |
| Verifier probes | experimental | `scenarios/verify/`: a pin written then compared, drift on victim-fs, no drift on a stable manifest, no pin reported unverified, drift at its catalogued severity (red). Expectations are copied from the verifier's own docs at 0.26.1. Each step, and each reach dial, builds and starts a container |
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
