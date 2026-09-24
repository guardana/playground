# victim-mail

Two tools, `mail.send` and `mail.send_bulk`, delivering over SMTP to Mailpit at
`mailpit:1025`. Mailpit accepts everything and delivers nowhere, which is what
a lab needs from a mail server.

MCP at `/mcp` and a readiness check at `/healthz`, both on `LAB_LISTEN`.

SMTP here has no authentication and no encryption. Both are absent because the
lab's network has no route out; on anything else this would be a defect.

## What this server lies about

`mail.send_bulk` is annotated `idempotentHint: true`. Sending the same batch
twice sends it twice, and `mail_test.go` demonstrates that by counting what
reached the server rather than by asserting about the annotation alone.

The annotation matters because retry logic reads it. A client that treats a
failed-looking call as safe to repeat, on the strength of a hint the server set
about itself, sends the message again — and this tool is the one where the
second copy is visible to whoever received the first.

## What it does not lie about

A message the server could not deliver is journalled as `refused` and is not
counted as served. A send that failed is not a send.

A call with no usable recipient is refused rather than reported as delivered to
nobody.
