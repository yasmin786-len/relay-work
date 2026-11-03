# Engineering Notes — Relay Campaign Events

## 1. My understanding

Relay receives provider webhooks when campaign messages are sent, delivered, opened, or clicked. Providers retry requests and can deliver events late or out of order. The service must accept batches, preserve valid events without counting retries twice, and provide campaign-level counts for the dashboard. It does not send marketing messages.

## 2. Assumptions

- `event_id` is globally unique across providers because the payload contains no provider identifier. It is the idempotency key.
- A repeated event ID with the same logical payload is a duplicate. Reuse of an ID with a different payload is rejected as a conflict; the first accepted event remains authoritative.
- `campaign_id`, `contact_id`, `event_id`, `type`, and `timestamp` are required and must be non-empty/valid.
- The four supported event types are exactly `sent`, `delivered`, `opened`, and `clicked`.
- Timestamps must be RFC3339 values with a timezone. They represent provider event time, not receive time.
- Metadata is optional and may contain any valid JSON value.
- Batch processing is per item: a malformed event does not discard valid siblings. The endpoint returns HTTP 200 for a syntactically valid batch and reports each item's outcome. A non-array or unreadable request is a 400.
- `sent`, `delivered`, `opened`, and `clicked` count accepted unique event records. `unique_opens` is additionally returned as a useful dashboard metric: distinct contacts with at least one accepted open per campaign.
- Repeated opens with different event IDs count as multiple `opened` events but one `unique_opens` contact.
- The service currently uses in-memory storage, which is explicitly allowed by the assignment. Data is lost on process restart; this is a documented trade-off for the time-boxed implementation.

## 3. Ambiguities

- Is `event_id` globally unique, or unique only within a provider? The payload has no provider ID.
- Should a reused event ID with different payload be treated as a duplicate, a conflict, or a provider-integrity error?
- Should dashboard opens mean total open events, unique contacts, or both?
- Should invalid records cause a whole batch to fail, or should valid records be accepted independently?
- Are multiple clicks and opens by the same contact meaningful events, and should they be retained individually?
- Should the dashboard show lifetime totals or support time-range filters and timezone selection?
- Are there provider-specific timestamp tolerances, event corrections, or deletion requirements?

## 4. Questions for the PM

1. Can you confirm the uniqueness scope of `event_id` and whether provider identity will be added?
2. Should a conflicting reuse of an event ID trigger an alert or only be reported to the caller?
3. Which definitions should the dashboard expose: total opens, unique opens, total clicks, unique clickers?
4. Do we need campaign date-range filters and a specified reporting timezone?
5. What are the provider retry/backoff expectations and maximum batch size?
6. What retention, privacy, and deletion requirements apply to contact-level events?
7. Is eventual consistency acceptable if ingestion is later moved behind a queue?

## 5. Priorities and implementation decisions

1. Correct per-event validation and partial batch processing.
2. Idempotency using event ID, including concurrent requests.
3. Campaign statistics from accepted events only.
4. Tests for duplicate delivery, partial batches, unique opens, and concurrent retries.
5. A small paginated recent-events endpoint after the required endpoints.
6. Clear run instructions and honest documentation of in-memory persistence.

The service serializes writes with a mutex and protects reads with an RWMutex. This is simple and race-safe for the assignment's current traffic, but it limits write parallelism and does not provide durability. The unique event ID map is the authoritative event store; statistics are derived from it rather than maintained as separate mutable counters, avoiding counter drift in this small implementation.

## 6. Scale memo — 100 million events/day

100 million/day averages roughly 1,157 events/second, with real traffic likely much peakier. The first concrete failure in this implementation is not CPU: it is the single in-memory process and its global write lock. Memory grows with every event, a restart loses all history, and a single process cannot provide durable high availability. Campaign stats and recent-event scans also become increasingly expensive because they iterate stored events.

I would change things in measured steps:

1. Establish load-test baselines and define latency/availability targets.
2. Move events to durable storage with a unique constraint on the agreed idempotency key; add indexes for `(campaign_id, timestamp)` and campaign/type aggregation.
3. Partition or shard by a key that preserves the uniqueness requirement, and verify how global event-ID uniqueness is enforced across partitions.
4. Add pre-aggregated campaign counters or time buckets, updated transactionally/idempotently, while retaining raw events for reconciliation.
5. Introduce horizontally scalable ingestion and workers only when measured load requires it.

The public API can remain `POST /events` and `GET /campaigns/{id}/stats`; response semantics should not change merely because storage changes. The storage design absolutely changes: an in-memory map is not durable or horizontally shared. A relational database may be a sensible next step; the exact choice should follow measured write/read patterns and operational constraints.

A queue (for example Kafka) can absorb bursts and decouple webhook latency from downstream aggregation. It creates new failure modes: at-least-once redelivery, consumer retries, poison messages, dead-letter handling, ordering/partition-key decisions, lag, replay, and a gap between webhook acceptance and dashboard visibility. A 2xx should mean the event has been durably accepted by the queue or database, not merely held in process memory.

Duplicates can reappear at every boundary: provider retries, API retry after a lost response, queue redelivery, worker crash after commit but before acknowledgement, replay/backfill, and dual-write/migration paths. Idempotency must therefore exist at the durable event store and in any counter projection, not just at the HTTP handler.

I would keep counters trustworthy by making event insertion idempotent, applying projection updates atomically with a processed-event marker or using an idempotent projection key, retaining raw events, and periodically reconciling aggregates from raw data. I would monitor ingestion rate, accepted/duplicate/rejected rates, p50/p95/p99 latency, HTTP errors, database write latency/locks, storage growth, queue lag, consumer retries/dead letters, projection lag, and reconciliation differences.

I would deliberately not introduce multi-region active-active processing, exactly-once claims, or complex streaming infrastructure before load tests and clear product requirements justify them.

## 7. Angry marketer investigation

Plausible explanations before declaring a bug:

- The dashboard query uses a rolling time window or different time boundaries between refreshes.
- A filter, campaign scope, timezone, or attribution rule changed.
- Open events were deduplicated or corrected during a backfill/reconciliation.
- Invalid or conflicting events were removed from a previously provisional count.
- The dashboard reads from a lagging or differently refreshed aggregate/materialized view.
- A cache was stale at 10:00 or 10:30.
- A data correction, provider suppression, privacy deletion, or retention job changed the underlying set.
- A race, lost update, or non-atomic projection update caused an inconsistent count.
- The UI displayed a partial response, stale component, or mismatched campaign/time filter.

I would first compare the exact API requests, filters, time boundaries, and response payloads from both refreshes. Then I would query the raw accepted events for the campaign, inspect event IDs and ingestion/processing logs, and compare raw-event aggregates with the dashboard projection. I would check whether any events were deleted/reclassified and whether the two reads came from the same consistent snapshot. If the contract says lifetime counts of immutable accepted events, opened should not decrease; a decrease would then indicate a bug or an explicit correction/deletion policy that must be visible. If it is a rolling-window or mutable reconciled metric, a decrease may be expected.

Delivered increasing is not suspicious by itself. A provider event may describe a delivery that happened earlier but arrived late, so the accepted delivered total can rise after the event is received. The important distinction is event time versus receive time.

## What I completed / what I intentionally skipped

Completed: Go HTTP service, batch ingestion, per-item validation/results, idempotency, conflicting-ID rejection, campaign stats including unique opens, paginated recent events, concurrency protection, tests, four debugging fixes, and written reasoning for scale and metric investigation.

Intentionally skipped: durable SQLite storage, authentication, provider-specific adapters, frontend/dashboard, queue, deployment, and advanced observability. In-memory persistence is allowed by the brief but is not durable; it is the most important limitation of this submission.

## If I had another day

I would replace the in-memory store with SQLite or PostgreSQL using a unique constraint and transactional inserts, add integration tests against the real database, add date-range filters and cursor pagination, run load tests, and add structured metrics/logging plus a reconciliation job.
