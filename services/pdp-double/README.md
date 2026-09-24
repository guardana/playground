# AuthZEN decision point double

It stands in for the AuthZEN decision point control's gateway asks about a call
a veto rule covers. It answers as a scenario's script says; a question no rule
matches is denied, never allowed.

It serves HTTPS only. At start it makes a CA and a serving certificate in memory
and writes the CA certificate alone to `-ca-out`; no key reaches disk. The file
appears only once the double is about to serve. The CA lives eight hours and
signs only server certificates for the `-san` names and addresses, and no
mailbox, URI or intermediate CA: its name constraints are critical, so a leaf it
signed for any other host does not verify. A `-san` under another is refused.

| path | answer |
|---|---|
| `POST <identifier path>/access/v1/evaluation` | the scripted answer |
| `GET /.well-known/authzen-configuration<identifier path>` | the metadata `doctor` compares |
| `GET /healthz` | `ok` |

Flags: `-identifier` (the `pdp.identifier` the gateway is configured with, an
`https` URL), `-san` (comma-separated names and IPs), `-ca-out`, `-script`,
`-listen` (default `:8443`), `-hold` (default `30s`, at most `60s`). Environment: `LAB_RUN_ID`
and `LAB_REPORTS_DIR`. It refuses to start when the identifier's host is not
among the `-san` names, since control would fail every handshake.

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
action asked about, `detail` the answer scripted for the question
(`unscripted` when nothing matched). What went out on the wire can differ: a
`timeout` ends as a 503 after `-hold` or with no answer once the caller leaves,
and a caller can leave before any answer reaches it; the journal has no second
line for either. A body it cannot read is recorded as `refused` under
`(unreadable)`, and a `POST` to any other path as `refused` under `(unrouted)`
with the method and the path as sent as `detail`, before its 404. A `tool` over
256 bytes or a `detail` over 2 KiB is cut and ends `[cut from N bytes]`, so one
request cannot make the journal unreadable. When the journal refuses a line,
the question is answered 503, never with a decision.
