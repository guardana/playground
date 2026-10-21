---
title: How a verifier scenario runs
summary: How the runner runs the pinned verifier against the victims and over the agent's own trace, and which file each verifier check reads.
type: explanation
audience: [engineering]
covers: [runner/verifier.go, runner/verifier_files.go, runner/token.go, runner/verifier_reach.go, runner/trace.go, runner/provenance.go, runner/check/verifier.go, runner/check/verifier_report.go, runner/check/verifier_verdict.go, runner/check/token.go, runner/check/authdouble.go, runner/check/effects_auth.go, runner/check/trace.go, runner/check/topology.go, agents/scripted/trace.go, compose/compose.yaml, compose/Dockerfile.verifier, compose/verifier/**, scripts/build-verifier.sh, versions.env, scenarios/verify/**, scenarios/trace/**, config/contracts/**, config/verifier/**, config/auth/**, services/auth-double/**, victims/vault*/**]
---

# How a verifier scenario runs

How the runner drives the verifier (`VERIFIER_PACKAGE` at `VERIFIER_VERSION`) in two shapes: probe steps against the victims, and the analysis of the agent's own trace against a contract.
For anyone reading a red verifier or trace check, or bringing a contract of their own.
The step and expectation formats are in [lab-files.md](../lab-files.md#verifier-scenarios); a run through the enforcer is in [scenario-run.md](scenario-run.md).

Both shapes are `experimental` ([status.md](../status.md)).

## Probe steps

A verifier scenario boots the `verifier` profile: the victims, `attacker-web`
and `mailpit`, without the enforcer. `victim-fs` rewrites `fs.read`'s
description on its second `tools/list` and keeps the new text. The enforcer
lists every victim's tools when it starts, so with it up, a `write_pin` step
would pin the rewritten description and a step compared against that pin
would see no drift.

```mermaid
sequenceDiagram
  participant runner
  participant verifier
  participant victim as victim-fs and the rest
  Note over runner: refuses a local image not labelled VERIFIER_VERSION
  runner->>verifier: docker compose run, dial each probed server
  runner->>verifier: dial 192.0.2.1 port 443, read the routing tables
  loop each step, in order
    Note over runner: makes verifier/step-n/in, copies the step's profile and pin into it
    runner->>verifier: docker compose run, probe --mcp http://victim-fs:8080/mcp --format json, with --mcp-pin and --profile from /lab-run/in, or --write-mcp-pin into /lab-run/out
    verifier->>victim: server/discover, initialize where expected, tools/list
    verifier->>victim: unauthenticated GETs of the two discovery documents
    victim-->>verifier: the tools as described now
    verifier-->>runner: exit code, the JSON report on standard output
    Note over runner: kept as verifier/step-n.json, a write_pin step's as step-n.stdout, standard error as step-n.stderr
  end
  Note over runner: grades each step, then every victim's journal
```

Sources: `runner/verifier.go`, `runner/verifier_reach.go`, `runner/provenance.go`.

Every step, and every reach dial, is a new container from the image
`make verifier-image` built. Compose never pulls that image, and the runner
refuses a local one whose `org.opencontainers.image.version` is not
`VERIFIER_VERSION`, before anything boots.

Each step sees one directory of the run, `verifier/step-<n>/`. Its `in/` is
bound read-only at `/lab-run/in` and holds what the runner handed the step: the
scenario's profile as `profile.yaml` and the pin an earlier step wrote as
`pin.json`, each copied as a regular file of at most 1 MiB, never through a
link. A `write_pin` step runs as the `verifier-pin` service, which also binds
the step's `out/` writable at `/lab-run/out`, where the pin lands. A step whose
inputs could not be handed over does not run, and neither does one handed the
pin of a step that did not exit 0. What a step prints is kept in
`verifier/` beside the step directories, which no container mounts, at a path
that does not exist yet. The victims' journals stay out of reach. The runner
reads every pin and report as a regular file of at most 8 MiB.

Each verifier run mints a token of 32 random bytes and puts it in compose's
environment as `LAB_MCP_TOKEN`, where the token-gated victims read it at boot.
A step with `credential` gets the variable by name (`-e LAB_MCP_TOKEN`) and
`--mcp-token-env LAB_MCP_TOKEN`, so the value is on no command line. Those
victims require it on `/mcp`, publish their protected-resource metadata naming
`auth-double`, and journal every authentication decision; `auth-double` serves
the scenario's persona and journals each request it gets, with whether it
carried a credential or a session and never their values.

## The agent's trace

A scenario with `trace:` runs through the enforcer as usual. After the replay,
the runner hands the agent's own record of the run to the verifier.

```mermaid
sequenceDiagram
  participant agent as scripted-agent
  participant runner
  participant tv as trace-verifier
  Note over agent: one span per step into agent/trace.jsonl
  agent-->>runner: exit status
  Note over runner: copies the trace into verifier/, a regular file of at most 8 MiB
  runner->>tv: docker compose run, analyze-trace /lab-run/trace.jsonl --contract /contracts/name.yaml --ai-system name --format json
  tv-->>runner: exit code, the JSON report on standard output
  Note over runner: kept as verifier/trace-report.json, graded as trace checks
```

Sources: `agents/scripted/trace.go`, `runner/trace.go`, `compose/compose.yaml`.

`trace-verifier` is alone on `trace-net` and mounts two directories read-only:
the run's `verifier/` and `config/contracts/`. It calls nothing. The trace is
the agent's account, so what it can and cannot tell is in
[lab-files.md](../lab-files.md#trace-scenarios); the scenario's decisions and
effects checks grade what the enforcer and the victims recorded.

## What each check reads

| check | passes when | source |
|---|---|---|
| `verifier/image` | never: it is the refusal of an image not built from the pin | the report |
| `compose/loaded` | never: it is the refusal of a run whose compose file did not load for its profile | compose's error, in the report |
| `boot/<service>` | every service of the profile runs | `boot.json` |
| `verifier-reach/<server>` | the verifier's network connects to each probed server | `probes.log` |
| `verifier-reach/no-route-out` | the dial to `192.0.2.1:443` ends in no route or an unknown name; a connection fails it, any other error is indeterminate | `probes.log` |
| `verifier-reach/no-default-route` | neither `/proc/net/route` nor `/proc/net/ipv6_route` in the verifier's container holds a usable default route | `probes.log` |
| `verifier-reach/no-proxy` | no `*_proxy` variable is set in the verifier's environment | `probes.log` |
| `verifier/step-<n>/ran` | never: it is the indeterminate of a step whose container did not start | the report |
| `verifier/step-<n>/pin` | a `write_pin` step left a pin of schema 2 that approves the probed URL and at least one tool | `verifier/step-<n>/out/pin.json` |
| `verifier/step-<n>/report` | any other step left a report of schema 18 about the probed URL, naming at least one rule that ran | `verifier/step-<n>.json` |
| `verifier/step-<n>/exit-code` | the exit code is the one stated; beside a missing record it is indeterminate, since a non-zero exit may be docker's | the pin or the report |
| `verifier/step-<n>/finding/<rule>` | a verifier finding of that rule with the stated severity and summary text | the report |
| `verifier/step-<n>/no-finding/<rule>` | the rule ran and left no verifier finding, waived one, unverified result or error | the report |
| `verifier/step-<n>/unverified/<rule>` | the rule is reported unverified | the report |
| `verifier/step-<n>/gate` | the report's own verdict, `run.result_summary.gate`, is the one stated; a report without it fails | the report |
| `verifier/step-<n>/shortfall` | `run.coverage.shortfall` holds exactly the stated gaps, one each, by kind and, where stated, name; a report without it fails | the report |
| `verifier/step-<n>/errors` | the report's `errors` name exactly the stated rules; a report without the list fails | the report |
| `effects/<victim>` | each probed server's journal holds what the scenario states, and every victim it does not name served nothing | `journals/<victim>/<victim>.jsonl` |
| `effects/<victim>/auth` | every decision in `auth_include` is among the auth lines the victim journalled in this run | `journals/<victim>/<victim>.jsonl` |
| `auth-double/unauthenticated` | with `auth_double` set, the double served the named persona to at least one metadata request in this run, and no request carried a credential, a session, a query string or a credential-like header name; none fetched is indeterminate | `journals/auth-double/auth-double.jsonl` |
| `verifier/token-kept-out` | no file under the run directory when the checks run holds the run's token in its name or its bytes; a file it could not read is indeterminate. The reports are written after it | every file under the run directory |
| `trace/*` | as the step checks of the same names, over a report whose target is this run's trace, `/lab-run/trace.jsonl#<run id>` | `verifier/trace-report.json` |

Exit codes 0 to 8 mean what `docs/exit-codes.md` in the verifier's repository
at `VERIFIER_VERSION` says; 2 is an unanswered question, never a clean result.

## Why it works this way

- **The exit code is graded only beside a record**
  (`runner/check/verifier.go`). A container that never reached the verifier
  exits non-zero too, and a 1 from docker is not the verifier's policy gate.
- **A report about another target is a record of something else**
  (`runner/check/verifier_report.go`): its schema, its target and its rules
  run are checked before any verifier finding is read.
- **`no-finding` means the rule ran and concluded**, not that its verifier
  finding is absent (`runner/check/verifier_report.go`). A rule that did not run, could
  not tell, or was waived has established nothing.
- **A step's inputs reach it read-only, in a directory of its own**
  (`runner/verifier_files.go`, `compose/compose.yaml`). The verifier writes only
  a pin step's `out/`, so it cannot change what a later step is handed or a
  report the runner grades.
- **An exit 2 says why** (`internal/labspec/verifier_rules.go`). The verifier
  exits 2 for a coverage gap, a pin it cannot use among them, for a rule that
  could not run, and under `fail_on_inconclusive` for a check that reached no
  verdict. A scenario states the gate, every gap and every error, and a
  control step through the same handover tells the verifier's verdict from the
  lab's own failure.
- **The token travels by name and is looked for afterwards**
  (`runner/token.go`, `runner/check/token.go`). A copy of it in a report, a
  journal or what a container printed fails the run, whoever wrote it.
- **Whether the token reached a victim is read from the victim**
  (`runner/check/effects_auth.go`): its own auth lines say what each request
  of the run carried. The victim cannot tell one step from another, so a step
  that never sent the token is caught by its exit code and gate, not here.
- **The victims' journals grade the verifier's own promise** to call no tool
  (`runner/verifier.go`, `everyVictimServesNothing`).
- **The verifier's network is proved sealed on every run**
  (`runner/verifier_reach.go`): a router past the network answers "no
  route" too, so the verifier's own routing tables are read as well. The
  probes run in `verifier`; `verifier-pin` is held to the same network and
  hardening by `compose/verifier_test.go`, and the teardown names its
  profile, so a pin step that outlived its deadline does not stay on
  `tool-net`.

## Where the code lives

| path | what |
|---|---|
| `runner/verifier.go` | the steps, the run's `verifier/` directory |
| `runner/verifier_reach.go` | the reach dials and the routing tables |
| `runner/trace.go` | the trace handed over and analysed |
| `runner/provenance.go` | the image refusal |
| `runner/check/verifier.go`, `runner/check/verifier_report.go`, `runner/check/trace.go` | the grading |
| `agents/scripted/trace.go` | the trace the agent writes |
| `compose/Dockerfile.verifier`, `compose/verifier/requirements.lock` | the verifier image, hash-locked |
