# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

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
