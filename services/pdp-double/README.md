# AuthZEN decision point double

It stands in for the AuthZEN decision point the enforcer asks about a call a
veto rule covers, and answers as a scenario's script says; a question no rule
matches is denied.

It serves HTTPS only, under a CA it makes in memory at start: only the CA
certificate is written, to `-ca-out`, once it is about to serve. The CA lives
eight hours and signs only server certificates for the `-san` names, under
critical name constraints, so a leaf for any other host does not verify.

| path | answer |
|---|---|
| `POST <identifier path>/access/v1/evaluation` | the scripted answer |
| `GET /.well-known/authzen-configuration<identifier path>` | the metadata `doctor` compares |
| `GET /healthz` | `ok` |

Flags: `-identifier` (the gateway's `pdp.identifier`, an `https` URL whose host
must be among the `-san` names), `-san`, `-ca-out`, `-script`, `-listen`
(`:8443`), `-hold` (`30s`, at most `60s`). Environment: `LAB_RUN_ID`,
`LAB_REPORTS_DIR`.

## Script

```yaml
schema_version: 1
rules:
  - match: {action: crm.refund, resource_type: order, resource_id: ord-1, subject_id: user-7}
    answer: allow
  - match: {action: crm.refund}
    answer: timeout
```

The first matching rule answers. `match` also takes `subject_type`; a key left
out matches anything, an unknown one is refused.

| answer | what control at `ENFORCER_COMMIT` records |
|---|---|
| `allow` | `PDP_ALLOW` |
| `deny` | `PDP_DENY` |
| `allow_obligation` | `OBLIGATION_NOT_UNDERSTOOD` |
| `timeout`: held until the caller leaves, or 503 after `-hold` | `PDP_TIMEOUT` |
| `status_500` | `PDP_UNAVAILABLE` |
| `no_echo`: an allow without `X-Request-ID` | `PDP_ANSWER_REFUSED` |
| `malformed`: truncated JSON | `PDP_ANSWER_REFUSED` |
| `extra_member`: an allow with an unknown top-level member | `PDP_ANSWER_REFUSED` |
| no rule matched: a deny saying nothing was scripted | `PDP_DENY` |

## Journal

Every question is recorded before it is answered, at
`${LAB_REPORTS_DIR}/${LAB_RUN_ID}/journals/pdp-double.jsonl`: `tool` is the
action asked about, `detail` the scripted answer (`unscripted` when nothing
matched). The journal records what was scripted, not what reached the caller: a
caller that leaves early gets no second line. An unreadable body is `refused`
under `(unreadable)`, a `POST` to another path `refused` under `(unrouted)`.
Long values are cut and marked. When the journal refuses a line, the question
is answered 503, never with a decision.
