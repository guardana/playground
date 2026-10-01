# Project rules

This is a laboratory. It exists to answer one question, for every release of the
systems it tests: do they actually block, approve, oblige, record and detect what
they claim to?

Two systems are under test. [Guardana](https://github.com/guardana/guardana)
verifies a deployed system and grades a run after the fact. The enforcement
plane decides one tool call as it is made, and records why. This repository
attacks both and checks the evidence they leave behind.

These rules bind everyone and everything that writes code here. Contributors
should also read CONTRIBUTING.md, which says the same things for people.

## The systems under test are not ours to edit

They are separate repositories, developed in parallel, and they move. Read them
if you need to; never change them from here, and never assume today's behaviour
matches yesterday's checkout.

What this repository tests is a pinned version — an image tag or a released
binary named in `versions.env`. A test that passes against whatever happened to
be on disk has measured nothing.

## Assert on evidence, never on narrative

An agent saying it refunded nothing is not proof that it refunded nothing. A
scenario passes on what the systems recorded and what the victim services show:
decisions, evidence records, spans, findings, and the state of the victim's own
data read back through its own API.

If a claim cannot be checked against a record, the scenario does not make it.

## INDETERMINATE is an outcome, and it is asserted

A scenario states which verdict it expects, including `INDETERMINATE`. An
`INDETERMINATE` where `ALLOW` or `DENY` was expected is a failure, never a pass,
unless that step is listed explicitly as tolerating it.

A missing evidence record is a failure. Unknown is never a pass.

## Deterministic first

Every scenario has a scripted trajectory that produces the same calls on every
run, with no model involved. A live model, when one is added (`planned`), is an
overlay: recorded to a cassette on first capture, replayed afterwards, and never
the thing that decides whether a build is green.

A scenario that cannot run reports that it could not run, and fails. It does not
skip quietly.

## The victims lie on purpose

The tool servers in `victims/` carry deliberately wrong annotations — a
destructive tool that claims to be read-only, a description that changes between
calls. That is the point: it proves annotations are hints and never
authorization. Do not fix them.

## Nothing here is real

Fixture data is synthetic. Canary tokens are planted so exfiltration is
detectable, and they lead nowhere. No real customer record, credential, key or
captured prompt enters this repository, in any fixture, at any time.

The attack payloads in `attacks/` are catalogued and inert: they exist to be
blocked by the system under test, inside a compose network with no route out.

## What belongs in this repository

The lab, and nothing else.

- Victim services, agents, the runner, scenarios, trajectories, attack payloads,
  policy bundles, compose files, benchmarks.
- Documentation a user or a contributor needs.

Not: planning documents, session notes, implementation reports, private specs,
scratch files, or captured output from a run. Results are written to `reports/`,
which is not tracked.

## Authorship

The work here belongs to the people who did it. No commit message, pull request,
code comment, changelog entry or document credits a tool.
`scripts/check-attribution.sh` enforces this for every file in the tree and
runs in `make quality`; a commit message is held to it in review.

## Language

English everywhere, including comments, commit messages and identifiers.

## Writing

Docs are short. A page answers one question. State what is true today; label
everything else `planned` or `experimental`.

Cut filler openings, restated summaries, adjectives that carry no information,
and paragraphs that narrate the code instead of explaining it. If a sentence
survives being deleted, delete it.

A comment explains why, or names an invariant, or records a constraint the code
cannot show. A comment that restates the line below it is noise.

Benchmark numbers are published with the machine that produced them, or not at
all.

## Code

- Go. `gofmt` decides formatting; there is nothing to discuss.
- Files under 250 lines. 350 warns, 500 fails without a written reason.
- Functions under 50 lines.
- No package named `utils`, `helpers`, `common`, `misc` or `shared`.
- No package-level mutable state.
- Anything crossing I/O takes a `context.Context` and carries a deadline.
- Malformed input returns an error. `panic` is for broken invariants only.
- A new dependency needs a paragraph in `docs/dependencies.md`: what it solves,
  why the standard library cannot, and its licence.

## Names are parameters

`versions.env` holds the image, tag and binary names of the systems under test.
Compose files and scenarios refer to them only through those variables, so
renaming a system upstream is one file here, not a search across the scenarios.

## Repeated work becomes a skill

Do the same multi-step thing twice and it stops being improvisation: write it
down, then run it from the written version. Never re-derive a procedure you have
already carried out.

## Commands

    make bootstrap       install and verify the pinned toolchain
    make quality-quick   format, vet, test, attribution, hygiene
    make quality         the full gate; CI runs exactly this

`make quality` is the definition of green. CI adds no check you cannot run
locally.

## Publishing

Work on a branch. Pushing, merging, tagging, creating the repository or changing
its settings are outward-facing and need the maintainer to say yes to that
specific action.

Commit with `git commit -s`. Subject as `type: what changed`.

Your own tooling stays out of the repository: `.git/info/exclude`, never
`.gitignore`. `scripts/check-hygiene.sh` fails if a hidden path becomes tracked.
