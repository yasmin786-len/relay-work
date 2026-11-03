# Relay Campaign Events API

A dependency-free Go HTTP service for ingesting provider campaign events and serving campaign analytics. Uses in-memory storage, which is permitted by the assignment; all data is lost when the process stops.

## Requirements

- Go 1.22+
- No third-party Go modules

## Run

```bash
cd starter
go test -race ./...
go run .
```

The API listens on `http://localhost:8080`. Set `PORT=18080` (or another port) if 8080 is already in use.

## Endpoints

### `POST /events`

Accepts a JSON array (maximum 1,000 events and 5 MiB). Each item is validated independently. Valid siblings are accepted even when another item is invalid. Event types: `sent`, `delivered`, `opened`, `clicked`. Timestamp must be RFC3339. Metadata can be any valid JSON value.

Example:

```bash
curl -i -X POST localhost:8080/events \
  -H 'Content-Type: application/json' \
  -d '[{"event_id":"evt_1","campaign_id":"cmp_demo","contact_id":"ct_1","type":"sent","timestamp":"2026-08-10T09:00:00Z"}]'
```

Response includes `received`, `accepted`, `duplicates`, `rejected`, and per-item results. Repeating the same event ID and logical payload returns `duplicate`. Reusing an event ID with different content returns `rejected` as a conflict. Event IDs are treated as globally unique.

### `GET /campaigns/{campaignID}/stats`

Returns total accepted unique event counts for the four event types and `unique_opens` (distinct contacts with at least one open in that campaign).

```bash
curl localhost:8080/campaigns/cmp_demo/stats
```

### `GET /campaigns/{campaignID}/events?limit=50&offset=0`

Optional paginated recent-event listing. Maximum limit is 200. Results are newest-first with event ID as a deterministic tie-breaker.

## Seed data

Start the server, then in another terminal:

```bash
curl -s -X POST localhost:8080/events \
  -H 'Content-Type: application/json' \
  --data-binary @seed/events.json
```

The seed contains malformed records and provider retries; the response reports accepted, duplicate, and rejected records independently.

## Design decisions and limitations

- `event_id` is the idempotency key. The in-memory map makes the check-and-insert atomic under a mutex.
- The service does not assume events arrive in order; statistics derive from accepted records.
- Total `opened` counts events; `unique_opens` counts distinct contacts per campaign.
- Reads use an RWMutex. Writes are serialized for correctness at this scale.
- Storage is in memory, so it is not durable and is not shared across multiple service instances. A production deployment should use durable storage with a unique constraint and transactional ingestion.
- There is no authentication, rate limiting, queue, or frontend, as these are outside the required scope.
