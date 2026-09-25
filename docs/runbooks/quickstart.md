---
title: Quickstart
summary: From a fresh clone to one scenario graded green against the pinned enforcer, what a red run means, and how to clean up.
type: runbook
audience: [engineering, product]
covers: [README.md, Makefile, versions.env, scripts/build-enforcer.sh, scripts/build-verifier.sh, scripts/lab-key.sh, runner/main.go, runner/report/**]
---

# Quickstart

## What you need

- Docker with Compose v2 and buildx. Docker Desktop on macOS and Docker Engine
  on Linux both work; on Linux, run as an ordinary user in the `docker` group.
- The Go version `go.mod` names, git, make and bash.
- A clone of the enforcer's repository that holds the commit `ENFORCER_COMMIT`
  in `versions.env`. The lab builds the enforcer from that commit with
  `git archive`, never from the clone's working tree, and refuses an archive
  whose tree is not `ENFORCER_TREE`. The public repository at
  `https://github.com/guardana/control` serves no ref for that commit today;
  until it does, ask the maintainers for a clone that holds it.
- Network for the first build: it pulls the base images and the verifier's
  Python packages, each pinned by digest or hash.

## Build the systems under test

```
git clone https://github.com/guardana/control "$HOME/control"
git clone https://github.com/guardana/playground
cd playground
export ENFORCER_SOURCE="$HOME/control"
make images
```

Any clone that holds the pinned commit will do; `ENFORCER_SOURCE` names it.

`make images` builds the enforcer image from the pinned commit and the
verifier image from its pinned release. Each prints the image it made. A
refusal says why: `ENFORCER_SOURCE` unset, the commit missing from the clone,
a tree that does not match.

## Make the lab key

```
make lab-key
```

The enforcer only runs a signed policy, so each run signs its scenario's
policy with a key the enforcer's own `policy keygen` made, once per machine. It
lives in `${XDG_STATE_HOME:-~/.local/state}/guardana-playground/lab-key`, or
where `LAB_KEYS_DIR` points; the lab refuses a key inside the clone, the
reports directory or a workspace. Running it again reports the key it has.

## Run one scenario

```
make scenario ID=tool-02-permitted-read-is-recorded-by-the-enforcer
```

The runner builds every lab image the scenario uses, boots its compose profile
in a project of its own, proves the topology, replays the trajectory, drains
the enforcer's trail to the collector, grades the run and takes the profile
down. The last line is the outcome, the scenario and its report:

```
pass       catalogue tool-02-permitted-read-is-recorded-by-the-enforcer  reports/<run id>/report.md
```

The first run builds every image and takes longest; later runs reuse them.

## Read the report

`reports/<run id>/report.md` opens with what produced the run: the lab commit,
the workspace, the machine, and each image with its ID and the pin its label
names. Then one row per check: what it wanted, what it got, and the record it
read. The records sit beside the report:

| file | what it holds |
|---|---|
| `evidence.jsonl` | the enforcer's trail, decoded from the collector's export |
| `journals/<server>.jsonl` | what each victim, and each double, did with every call it received |
| `healthz.json` | the enforcer's `/healthz` answer after the replay |
| `probes.log`, `boot.json` | the topology probes and what came up |
| `plane.log` | the enforcer's version, image and drain |
| `junit.xml` | the same results for a CI system |

`make scenarios` runs the whole catalogue. Two scenarios are red by design,
each on a finding in a system under test; `scenarios/red-by-design.txt` names
them and the finding, and `go run ./runner -all -red-by-design
scenarios/red-by-design.txt` passes only when the reds are exactly those.

## When it is red

A red check names the record it read and what it found there; open that file.
A check is `indeterminate` when it could not read its record at all, which
fails the run as surely as a wrong verdict. Common causes:

- `plane/prepared` failed: no lab key, or the policy did not sign; the detail
  says which.
- `boot/*` failed: a service did not start. `boot.json` carries compose's own
  reason, a subnet another run is using included.
- `plane/image` failed: the enforcer image was not built by `make images` from
  the pin. Run `make images` again.
- `plane/drained` failed: the trail did not reach the collector in time; the
  machine was likely busy. Run the scenario again before reading anything into
  it.

## Clean up

A run takes its compose project down, volumes included, unless you passed
`-keep` to the runner. What stays is yours to remove:

```
rm -rf reports
docker image ls 'playground-*'
```

The lab key stays where `make lab-key` put it.
