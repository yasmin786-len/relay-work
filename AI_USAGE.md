# AI Usage Disclosure

## Tools used
- ChatGPT for interpreting the brief, reviewing design trade-offs, generating an initial implementation outline, and helping explain Go concurrency and idempotency.
- Go toolchain (`go test`, `go test -race`, `go vet`, `go run`) for local verification.

## Roughly what AI was used for
- Breaking the assignment into deliverables and prioritizing within the time box.
- Discussing partial batch acceptance, event identity, late/out-of-order events, and concurrency safety.
- Reviewing the debugging program and explaining the root causes of its four defects.
- Drafting documentation that I reviewed and should be prepared to explain.

## One suggestion I changed or rejected
A SQLite implementation was considered, but dependency download was unavailable in the working environment. The assignment explicitly permits in-memory persistence, so I chose a dependency-free in-memory store rather than introduce an unverified external dependency. This means data is lost on restart, and that limitation is clearly documented.

## One thing AI helped me understand
A provider event's timestamp is not the same as its receive time. Events can arrive late and out of order, so aggregation must not rely on arrival sequence. Event IDs provide idempotency across retries.

## AI-generated code that required verification/debugging
The initial service implementation and debugging changes were checked with the Go compiler, tests, race detector, and exact expected-output comparison. I corrected the event metadata representation to use `json.RawMessage` so arbitrary JSON metadata is preserved rather than interpreted as a base64-encoded byte slice.
