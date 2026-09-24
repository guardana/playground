# Approver

It stands in for the person who answers a call the plane held for approval. It
never writes an approval record: every answer is the enforcer's own
`approvals approve` or `approvals reject`, run from the enforcer image built at
`ENFORCER_COMMIT`, which the approver's image carries at `/enforcer/control`.

Every `-interval` (default `250ms`) it runs `approvals list <dir>` and deals with
each approval still waiting (state `APPROVAL_STATE_PENDING`, resolution
`pending`) once. A listing that exits non-zero or does not parse is an error,
never "nothing waits". While the listing says no plane holds the directory,
nothing is answered: the command would write the answer and exit 0, and the
held call would still never run. `/healthz` answers 200 only while the last
listing went through and a plane held the directory.

Flags: `-dir`, `-script`, `-control` (default `/enforcer/control`), `-listen`
(default `:8080`), `-exec-timeout` (default `10s`). Environment: `LAB_RUN_ID`
and `LAB_REPORTS_DIR`.

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
values; an absent key matches anything, an unknown one is refused, and so is an
`effect_class` the listing never prints. A record the listing shows no readable
fields for matches only a rule whose `match` is `{unreadable: true}`, never a
catch-all. The listing
does not show the upstream, so no rule can match on it. `delay` counts from the
first listing that showed the approval. `approver_id` and `reason` reach the
command as written, so a script can make it refuse.

## Journal

`$LAB_REPORTS_DIR/$LAB_RUN_ID/journals/approver.jsonl`, server `approver`. A run
id with a path separator or `..` is refused at start.

| tool | status | detail |
|---|---|---|
| `approve`, `reject` | `served` on exit 0, else `refused` | `approval=<id> exit=<status>`, `none` when the command did not start |
| `unknown` | `served` | `approval=<id> answer=<answer>`: the command was killed at its deadline or at shutdown, so it may have written |
| `approve`, `reject`, `unknown` | as below | after `unknown`, what the next listing shows: `record=<state>/<resolution>` and `answered_by=<id>` |
| `wait` | `served` | `approval=<id> rule=<n> delay=<delay>`, at the first sighting of a delayed answer |
| `no-plane` | `served` | `approval=<id>`, once, for an approval seen while no plane held the directory; it is never answered |
| `leave` | `served` | `approval=<id> rule=<n>`, `approval=<id> unmatched`, or `approval=<id> unmatched unreadable` |

After an `unknown`, the record decides the next line: `served` under the answer
when it carries that answer from the rule's `approver_id`, `refused` when nobody
answered it, and `unknown` with `record=absent` or whatever else it shows. One
last listing is read at shutdown for this. A refused answer is not retried.
