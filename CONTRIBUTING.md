# Contributing

This repository attacks security tooling on purpose. A scenario that passes for
the wrong reason is worse than no scenario, so most of the review effort goes
into whether an assertion could pass while the thing it claims to test is
broken.

## Before you write code

Search the issues first. A new scenario needs an identifier and a statement of
what failure it would catch. "More coverage" is not that statement.

Keep the scope small. Unrelated cleanup belongs in its own change.

## Working locally

```
make bootstrap
make quality
```

`make quality` is the whole gate, and CI runs the same targets. CI's scenario
job runs `scripts/ci-scenarios.sh`, which you can run the same way with Docker
and `ENFORCER_SOURCE` set.
`make docs-impact FOR=<path>` names the pages a change to that path makes suspect.

Local tooling — editor settings, agent configuration, scratch notes — stays out
of the repository. Put it in `.git/info/exclude` rather than in `.gitignore`, so
your setup does not become everyone's.

## What review looks for

- The assertion reads a record, not a claim. Decisions, evidence, spans, or the
  victim service's own state — never what the agent said it did.
- The scenario fails for the right reason. Break the thing under test on purpose
  and show the scenario going red.
- `INDETERMINATE` is stated, not tolerated by accident.
- A victim's wrong annotation is deliberate and stays wrong.
- No real data. Fixtures are synthetic and canaries lead nowhere.
- Documentation changed in the same commit as the behaviour it describes.

## Commits

Sign off with `git commit -s`. That is the Developer Certificate of Origin: you
are saying you have the right to contribute the code. There is no separate
agreement to sign.

Write commit subjects as `type: what changed` — `feat`, `fix`, `security`,
`perf`, `refactor`, `docs`, `test`, `build`, `ci`.

No commit, pull request, comment or document credits a tool for the work.
`scripts/check-attribution.sh` enforces that, and it runs in the gate.

## Reporting a vulnerability

Not here. `SECURITY.md`.
