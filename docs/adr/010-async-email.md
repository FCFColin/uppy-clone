# ADR-010: Asynchronous Email Sending

## Status
Accepted

## Date
2026-06-23

## Context
Magic link emails were sent synchronously in the request handler, causing:
- Request latency dependent on SMTP/HTTP API latency (100ms-5s)
- Request failures when email API is down
- No retry mechanism for transient failures

## Decision
Use Redis Stream as a message queue for asynchronous email sending:
1. Request handler enqueues email payload to `email:queue` stream
2. Background EmailWorker consumes via XREADGROUP (consumer group)
3. Failed sends retry up to 5 times, then move to `email:dead-letter` stream
4. Handler returns 202 Accepted immediately

## Consequences
**Positive:**
- Request latency no longer blocked by email API
- Automatic retry with exponential backoff
- Dead-letter queue for manual investigation
- Horizontal scaling: multiple workers can join the consumer group

**Negative:**
- Additional Redis storage for queue
- No immediate feedback if email send fails
- Requires monitoring of queue depth and dead-letter queue

## Alternatives Considered
1. **Synchronous with timeout**: Still blocks request, no retry
2. **Goroutine + channel**: No persistence, lost on restart
3. **External MQ (RabbitMQ/Kafka)**: Overkill for email volume
