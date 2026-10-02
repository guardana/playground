# Guardana Playground

A lab for finding out whether agent security tooling does what it says.

Two systems are under test. [Guardana](https://github.com/guardana/guardana),
the verifier, grades recorded runs;
[Control](https://github.com/guardana/control), the enforcer, decides tool
calls inline. The lab replays scripted agent trajectories against deceptive
tool servers and injected content, then checks both systems' records and the
victims' effects.

Maintainers test releases; adopters test their own policy, gateway
configuration or verifier contract. See the [use cases](docs/reference/use-cases.md)
and [failure modes](docs/reference/failure-modes.md).

**Status: experimental.** The scenario catalogue runs against the enforcer and
the verifier at the versions `versions.env` pins. [Status](docs/status.md) says
what exists, component by component, and which scenarios are red on purpose.

Planned: Range will run an external agent against the same victims and grade
its observed effects. The enforcer and the verifier remain optional. The
[roadmap](ROADMAP.md) also covers model artifacts and endpoints.

## One green scenario

You need Docker with Compose v2 and buildx, Go, git and make. The enforcer is
pinned to a public release, which `scripts/fetch-enforcer.sh` fetches
anonymously:

```
git clone https://github.com/guardana/playground
cd playground
scripts/fetch-enforcer.sh /path/to/control.git
export ENFORCER_SOURCE=/path/to/control.git
make images
make lab-key
make scenario ID=tool-02-permitted-read-is-recorded-by-the-enforcer
```

Until the first release is published there, the repository is empty; use the
checkout you have.

Its result line gives the verdict and report path. The
[quickstart](docs/runbooks/quickstart.md) covers setup and failures.

`make smoke` (experimental) runs five green paths across both systems and
fails on any red or unrunnable path. `make scenarios` runs the whole catalogue
and is red on the scenarios [red by design](scenarios/red-by-design.txt).

## Your own policy, configuration or contract

Put them in a directory outside the clone, laid out like the lab, and run them
with `LAB_WORKSPACE`. `examples/helpdesk-payouts/` is a worked one. The runbooks
say what each file is and what the lab keeps for itself:

- [bring your own policy](docs/runbooks/bring-your-own-policy.md)
- [bring your own gateway configuration](docs/runbooks/bring-your-own-gateway-configuration.md)
- [bring your own verifier contract](docs/runbooks/bring-your-own-contract.md)

## How a run is graded

A scenario names its trajectory and expected verdict. The runner grades from
the enforcer's exported trail, victim journals, `/healthz` and the verifier's
report. The agent's account of its work is not evidence.

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

`versions.env` pins both systems and every image the lab runs;
[dependencies](docs/dependencies.md) says how each is built and checked.

## Contributing

`make bootstrap` installs the gate's tools and checks each against its pin,
`make quality` is the gate CI runs.
`CONTRIBUTING.md` has the short version, `AGENTS.md` the same rules written for
the tools people point at this repository.

Apache-2.0. Sign your commits with `git commit -s`.
