# Approver

It stands in for the person who answers a held call. Every answer is the
enforcer's own `approvals approve` or `approvals reject`, from the enforcer
image built at `ENFORCER_COMMIT` (`/enforcer/control`); it writes no record
itself.

Every `-interval` (`250ms`) it runs `approvals list <dir>` and deals once with
each approval still pending. A listing that fails or does not parse is an
error, never "nothing waits". While no plane holds the directory, nothing is
answered: the answer would be written and the held call would never run.
`/healthz` is 200 only while listings go through and a plane holds the
directory.

Flags: `-dir`, `-script`, `-control`, `-listen` (`:8080`), `-exec-timeout`
(`10s`). Environment: `LAB_RUN_ID`, `LAB_REPORTS_DIR`.

## Script

```yaml
schema_version: 1
rules:
  - match: {action: refund, resource: payment pay-9}
    answer: approve
    approver_id: lab-approver
    reason: scripted
    delay: 2s
  - match: {effect_class: EFFECT_CLASS_DELETE}
    answer: reject
    approver_id: lab-approver
  - match: {}
    answer: leave
```

The first matching rule answers. `match` takes `action`, `resource`,
`effect_class`, `principal` and `agent`, compared with the listing's printed
values; an absent key matches anything, an unknown key or an `effect_class` the
listing never prints is refused. An unreadable record matches only
`{unreadable: true}`, never a catch-all. The upstream is not listed, so no
rule matches it. `delay` counts from the first listing that showed
the approval.

## Journal

`$LAB_REPORTS_DIR/$LAB_RUN_ID/journals/approver.jsonl`, server `approver`.

| tool | status | detail |
|---|---|---|
| `approve`, `reject` | `served` on exit 0, else `refused` | `approval=<id> exit=<status>`, `none` when the command did not start |
| `unknown` | `served` | `approval=<id> answer=<answer>`: the command was killed, so it may have written |
| `approve`, `reject`, `unknown` | as below | after `unknown`: `record=<state>/<resolution>` and `answered_by=<id>` |
| `wait` | `served` | `approval=<id> rule=<n> delay=<delay>`, at the first sighting of a delayed answer |
| `no-plane` | `served` | `approval=<id>`, once, seen while no plane held the directory; never answered |
| `leave` | `served` | `approval=<id> rule=<n>`, `approval=<id> unmatched`, or `approval=<id> unmatched unreadable` |

After an `unknown`, the next listing decides the line: `served` when the record
carries that answer from the rule's `approver_id`, `refused` when nobody
answered, `unknown` otherwise; one last listing is read at shutdown. Refusals
are not retried.
