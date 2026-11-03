# Relay Campaign Events — Engineering Assignment Submission

This repository contains the Go campaign-event ingestion service, the supplied debugging exercise with four minimal fixes, and the written engineering notes.

## Quick Start

### 1. Run the Go service

```bash
cd starter
go test -race ./...
go run .
```

The API listens on port `8080` by default.

If port `8080` is already in use, choose another port:

```bash
PORT=18080 go run .
```

See [`starter/README.md`](starter/README.md) for endpoint examples and design details.

## API Endpoints

The service provides the following endpoints:

- `POST /events` — Accepts a JSON array of campaign events and validates each event independently.
- `GET /campaigns/{campaign_id}/stats` — Returns campaign event counts and unique opens.
- `GET /campaigns/{campaign_id}/events` — Returns paginated campaign events.

The implementation handles duplicate event IDs, invalid events, late and out-of-order events, and concurrent requests.

## Verify the Debugging Exercise

Open another terminal and run:

```bash
cd ~/Downloads/relay-work/debugging

go run . events.jsonl > actual.txt
diff -u expected_output.txt actual.txt

go run -race . events.jsonl > actual-race.txt
diff -u expected_output.txt actual-race.txt
```

Both `diff` commands should produce no output if the program matches the expected results. The race detector should report no data races.

After verification, remove the generated output files:

```bash
rm -f actual.txt actual-race.txt
```

## Repository Contents

- `NOTES.md` — Problem interpretation, assumptions, ambiguities, product-manager questions, priorities, scalability memo, angry-marketer investigation, and completion notes.
- `starter/` — Go HTTP service, seed data, tests, and run instructions.
- `debugging/` — Debugging exercise source code, input events, expected output, and fixes.
- `BUGS.md` — Explanation of the four debugging issues, their causes, fixes, and verification.
- `AI_USAGE.md` — Disclosure of AI tools used and how their suggestions were evaluated.
- `ASSIGNMENT.md` — Original engineering assignment.

## Design and Implementation

The service uses Go's standard library and an in-memory event store protected by synchronization.

Key implementation details:

- Per-event validation and partial batch acceptance.
- Duplicate event detection and idempotent handling.
- Campaign statistics calculated from accepted events.
- Unique opens calculated using distinct contacts per campaign.
- Paginated campaign event retrieval.
- Thread-safe access to shared event data.

## Important Limitation

The service uses in-memory persistence, which the assignment explicitly allows.

This demonstrates event ingestion and correctness behavior, but it is not durable: restarting the process clears accepted events. Persistent storage and production hardening are documented as future improvements.

## Testing

Run the service tests:

```bash
cd starter
go test -count=1 -race ./...
go vet ./...
```

Run the debugging verification commands from the section above to compare the program output with the expected output and check for data races.
