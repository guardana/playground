# Status

What exists, stated once, so no other page has to guess.

Labels: `implemented` runs and is tested · `experimental` runs, may change
without notice · `planned` does not exist.

| Component | Status | Notes |
|---|---|---|
| Repository rules and quality gate | implemented | `make quality` |
| Pinned versions of the systems under test | implemented | `versions.env` |
| Trajectory and scenario file formats | implemented | `internal/labspec`; an unknown key is refused and the two files cross-validate |
| Evidence, journal and assertion readers | implemented | `internal/evidence`, `internal/journal`, `internal/assertion` |
| Compose topology | implemented | `compose/`; ten services on two networks, `tool-net` with no route out |
| Stub gateway | experimental | replays the declared verdicts in `config/scenarios/`; it decides nothing. It lists an upstream's tools once at startup as well as on demand, so a victim counting its own listings is one ahead of the agent |
| Victim tool servers | implemented | all six: crm, db, fs, shell, mail, web; each lies in the way its README states |
| Scripted agent | implemented | `agents/scripted`; replays a trajectory and forwards each step's output into the next |
| Scenario runner and assertions | implemented | `make scenario ID=...`; boot, topology, replay, decisions, effects and evidence checks |
| Scenario catalogue | experimental | three scenarios, one `ALLOW`, one `DENY`, one `INDETERMINATE`; they have run only against the stub |
| Attack payload catalogue | experimental | four indirect injections in `attacks/`, served by `attacker-web` |
| Chaos matrix | planned | latency, timeouts, cut links, full disk, clock skew |
| Verifier loop | planned | probe, analyse the evidence, gate on a regression |
| Live model overlay | planned | recorded to cassettes, replayed in CI |
| Benchmarks | planned | decision latency against policy bundle size |

The three scenarios run end to end and pass, and each was made to fail on
purpose by changing what the stub replays. That is what a green run here means:
the runner asserts what it claims to assert. It says nothing about the
enforcement plane, because no scenario has run against anything that decides —
the verdicts come from `config/scenarios/<id>.yaml`, written by the scenario's
author. Reading a green catalogue as evidence about the enforcement plane would
be the mistake this lab exists to catch.

Nothing has been measured yet. Any number this repository publishes later comes
with the machine that produced it.
