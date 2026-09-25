# Guardana Playground

A lab for finding out whether agent security tooling does what it says.

Two systems are under test. [Guardana](https://github.com/guardana/guardana)
grades a run after it happened. The enforcement plane decides one tool call as
it is made. This repository builds a small, hostile world around both of them —
tool servers that lie about what they do, web pages carrying injected
instructions, a decision point that stops answering mid-run — replays scripted
agent trajectories through it, and checks the records they left behind.

It serves two readers: the maintainers, who run it against every release of
both systems, and a team deploying either one, who runs its own policy, gateway
configuration or verifier contract here before production.
[docs/reference/use-cases.md](docs/reference/use-cases.md) lists the agent
deployments it simulates and the [failure modes](docs/reference/failure-modes.md)
each one meets.

**Status: experimental.** The catalogue of 34 scenarios runs against the
enforcer and the verifier at the versions `versions.env` pins; two of them are
red on purpose, each on a recorded finding. `docs/status.md` says what exists,
component by component.

## One green scenario

You need Docker with Compose v2 and buildx, the Go version `go.mod` names, git,
make, and a clone of the enforcer that holds the commit `versions.env` pins
(the quickstart says where to get one):

```
git clone https://github.com/guardana/playground
cd playground
export ENFORCER_SOURCE="$HOME/control"
make images
make lab-key
make scenario ID=tool-02-permitted-read-is-recorded-by-the-enforcer
```

The last line printed is `pass`, the scenario and the path of its `report.md`.
[docs/runbooks/quickstart.md](docs/runbooks/quickstart.md) walks through each
step, what a red run means, and how to clean up.

## Your own policy, configuration or contract

Put them in a directory outside the clone, laid out like the lab, and run them
with `LAB_WORKSPACE`. `examples/helpdesk-payouts/` is a worked one. The runbooks
say what each file is and what the lab keeps for itself:

- [bring your own policy](docs/runbooks/bring-your-own-policy.md)
- [bring your own gateway configuration](docs/runbooks/bring-your-own-gateway-configuration.md)
- [bring your own verifier contract](docs/runbooks/bring-your-own-contract.md)

## How a run is graded

A scenario names a trajectory, what decides it, and what it expects. The
runner boots the profile, replays the trajectory, then reads the answer from
records that cannot be talked into lying: the trail the enforcer exported, the
victim services' own journals, the enforcer's `/healthz`, and the verifier's
report. What the agent said about its work is not evidence.

A verdict of `INDETERMINATE` where `ALLOW` or `DENY` was expected fails the run.
A missing record fails the run. Unknown is never a pass. A green scenario is
evidence about that path at those pins and nothing wider.

## What is deliberately broken

The tool servers in `victims/` carry wrong annotations on purpose: a payout
change that claims to be read-only, a server whose tool descriptions change
between calls. That is how the lab proves annotations are hints and never
authorization. They are not bugs and they do not get fixed.

Nothing in here is real. Fixture data is synthetic, canary tokens lead nowhere,
and the attack payloads sit on compose networks with no route out.

## Versions

`versions.env` pins the enforcer by commit and tree, the verifier by release
with every dependency hash-locked, and every other image by digest. A run
against whatever happened to be on disk has measured nothing.

## Contributing

`make bootstrap` installs the pinned tools, `make quality` is the gate CI runs.
`CONTRIBUTING.md` has the short version, `AGENTS.md` the same rules written for
the tools people point at this repository.

Apache-2.0. Sign your commits with `git commit -s`.
