# Status

What exists, stated once, so no other page has to guess.

Labels: `implemented` runs and is tested · `experimental` runs, may change
without notice · `planned` does not exist.

| Component | Status | Notes |
|---|---|---|
| Repository rules and quality gate | implemented | `make quality` |
| Pinned versions of the systems under test | implemented | `versions.env`, every pulled image by tag and index digest; `make images` builds the enforcer from `git archive` of its commit, refused unless the archive hashes to that commit's tree, and the verifier from its hash-locked release; every report prints the pins, the machine, and each image on this machine with its ID and the pin its label names. The verifier image runs in the `verifier` profile; no service runs the enforcer image yet |
| Trajectory and scenario file formats | implemented | `internal/labspec`; an unknown key is refused and the two files cross-validate. Steps pair with the trails they open by order and by tool, can wait, retry a held call, resume a held trail or open none; named gaps are their own suite. A verifier scenario has probe steps instead of a trajectory. The trail cannot tell which retry resumed a hold. Nothing has graded a real enforcer trail yet |
| Evidence, journal and assertion readers | implemented | `internal/evidence` mirrors the enforcer's v1 contract at its pinned commit and reads its OTLP log export; `internal/journal`, `internal/assertion`. No scenario has read a real export yet |
| Documentation checks | experimental | `make docs-check` (in the gate): local links over the files the repository lists, the index `docs/README.md` current, the frontmatter check and both scripts compiled. `make docs-frontmatter` (not in the gate yet; red until the pages are migrated) checks frontmatter, budgets and `covers` from `docs/docs.json`. `make docs-impact` names the pages a change makes suspect |
| Compose topology | implemented | `compose/`; eleven services on two networks, `tool-net` with no route out; the `verifier` profile runs the victims without the gateway |
| Stub gateway | experimental | replays the declared verdicts in `config/scenarios/`; it decides nothing. It lists an upstream's tools once at startup as well as on demand, so a victim counting its own listings is one ahead of the agent |
| Victim tool servers | implemented | all six: crm, db, fs, shell, mail, web; each lies in the way its README states |
| AuthZEN decision point double | experimental | `services/pdp-double`: HTTPS with a CA it makes in memory and scopes to its own names, answers scripted per scenario (allow, deny, obligation, timeout, 500, no echo, malformed, extra member; unscripted is denied), every question journalled. No scenario uses it yet |
| Approver | experimental | `services/approver`: answers held approvals per scenario script through the enforcer's own `approvals` command, never while no plane holds the directory; every action and every outcome it cannot confirm journalled. No scenario uses it yet |
| Scripted agent | implemented | `agents/scripted`; replays a trajectory and forwards each step's output into the next |
| Scenario runner and assertions | implemented | `make scenario ID=...`; boot, topology, replay, decisions, trails, effects and evidence checks; a verifier scenario is graded on boot, the verifier's reach and routing tables, each step's exit code, pin and JSON report, and every victim's journal; every run in a compose project of its own |
| Scenario catalogue | experimental | three enforcer scenarios, one `ALLOW`, one `DENY`, one `INDETERMINATE`, run only against the stub; four verifier scenarios in `scenarios/verify/`, run against the verifier at 0.26.1, one of them red (below) |
| Attack payload catalogue | experimental | four indirect injections in `attacks/`, served by `attacker-web` |
| Chaos matrix | planned | latency, timeouts, cut links, full disk, clock skew |
| Verifier probes | experimental | `scenarios/verify/`: a pin written then compared, drift on victim-fs, no drift on a stable manifest, no pin reported unverified, drift at its catalogued severity (red). Expectations are copied from the verifier's own docs at 0.26.1. Each step, and each reach dial, builds and starts a container |
| Verifier loop | planned | analyse the evidence of a run through the enforcer, gate on a regression |
| Live model overlay | planned | recorded to cassettes, replayed in CI |
| Benchmarks | planned | decision latency against policy bundle size |

The three scenarios run end to end and pass, and each was made to fail on
purpose by changing what the stub replays. That is what a green run here means:
the runner asserts what it claims to assert. It says nothing about the
enforcement plane, because no scenario has run against anything that decides —
the verdicts come from `config/scenarios/<id>.yaml`, written by the scenario's
author. Reading a green catalogue as evidence about the enforcement plane would
be the mistake this lab exists to catch.

The verifier scenarios run the verifier's `probe` at its pin against the
victims. Three pass; each was made to fail on purpose by changing the steps, not
the expectations (a pin taken after the drift, a drifting server where a stable
one was expected, a pin where none was expected). Their green is evidence about
the verifier's MCP manifest checks at 0.26.1 and nothing wider.
`verify-04-drift-severity-is-the-catalogued-one` is red, and stays red until the
verifier or its rule catalogue changes: the catalogue lists
`guardana.agent.mcp_server_manifest` as `HIGH` and the verifier reports the drift
`CRITICAL`, so `make scenarios` is red on it.

Nothing has been measured yet. Any number this repository publishes later comes
with the machine that produced it.
