# ADR-009: Transactional Outbox Pattern

## Status
Accepted

## Date
2026-06-23

## Context
When creating users, we need to publish a "user.created" event to Redis Stream for downstream consumers (email, analytics). However, writing to PostgreSQL and Redis is not atomic — if the Redis publish fails after the PG insert, the event is lost.

## Decision
Use the Transactional Outbox pattern:
1. Write the business data AND the outbox event in the same PostgreSQL transaction
2. A background Publisher goroutine polls outbox_events every 1 second
3. Unprocessed events are published to Redis Streams
4. Successfully published events are marked with processed_at timestamp

## Consequences
**Positive:**
- Atomicity: business data and event are committed together
- At-least-once delivery: events are never lost
- Replayability: unprocessed events remain in the table
- Ordering: events are processed in ID order

**Negative:**
- Additional DB polling (mitigated by 1s interval + LIMIT 100)
- Potential duplicate publishing (consumers must be idempotent)
- Requires monitoring of unprocessed event count

## Alternatives Considered
1. **2PC (Two-Phase Commit)**: Not supported by Redis, adds latency
2. **Change Data Capture (CDC)**: Complex to set up (Debezium), overkill for current scale
3. **Best-effort publishing**: Unreliable, events can be lost
