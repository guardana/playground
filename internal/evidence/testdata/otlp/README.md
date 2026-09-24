# OTLP logs golden from the enforcer

`events.jsonl` and `export_logs_request.json` are byte-for-byte copies of
`testdata/otlp/events.jsonl` and `testdata/otlp/export_logs_request.json` in
the enforcer's source tree at commit
`e72ebe261af4ca9bb4b683ddedda81bfcc5de906`. The enforcer's exporter test holds
its own output to this pair, so decoding the request here and matching the
four events is the lab reading the same bytes the exporter is checked against.

Neither file is edited by hand. When the pin moves, copy both again from the
new commit and change the commit above in the same change.
