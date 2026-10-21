# Authorization server double

It stands in for the authorization server a protected MCP server names in its
resource metadata, at `http://auth-double:8080`, so the verifier's MCP probe
has a metadata document to check. It serves one document, the persona a
scenario chose, and nothing else: no authorization or token endpoint answers.

It is hostile by default. With no persona, or one that does not load, it stays
up and answers 404 to every metadata request, so a run that names none still
boots and a check that needs the document has nothing to read.

| path | answer |
|---|---|
| `GET /.well-known/oauth-authorization-server` | the persona, byte for byte, as `application/json`; 404 without one |
| `GET /healthz` | `ok` |
| anything else | 404 |

Flags: `-personas` (the directory holding the persona files), `-listen`
(`:8080`). Environment: `LAB_AUTH_PERSONA` (the persona's name, none when
unset), `LAB_RUN_ID`, `LAB_REPORTS_DIR`.

## Personas

A persona is `config/auth/<name>.json`, the RFC 8414 metadata document to
serve. The double serves it as written, so a persona can omit or change any
field; a file that is not JSON does not load. A name is lower-case letters,
digits and inner hyphens.

| persona | what it says |
|---|---|
| `honest` | issuer `http://auth-double:8080`, the URL the metadata is fetched from; PKCE `S256`; `iss` in the authorization response; scopes `vault.read`, `vault.write` |
| `no-pkce` | `honest` without `code_challenge_methods_supported` |

## Journal

Every request is recorded before it is answered, at
`${LAB_REPORTS_DIR}/${LAB_RUN_ID}/journals/auth-double.jsonl`, one JSON object
per line in this double's own format:

```json
{"occurred_at":"…","server":"auth-double","run_id":"…","method":"GET","path":"/.well-known/oauth-authorization-server","authorization":true,"mcp_session_id":false,"status":200,"persona":"honest"}
```

`authorization` and `mcp_session_id` say whether the request carried that
header; a header's value is never written. `persona` is absent when none
loaded. The health probe the container runs on loopback is not recorded; a
`/healthz` from the network is. Long methods and paths are cut and marked.
When the journal refuses a line, the request is answered 503, never with the
document.
