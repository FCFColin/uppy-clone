-- Transactional Outbox for reliable event publishing
-- 企业为何需要：跨数据源（PG+Redis）的原子性无法用分布式事务保证。Outbox 模式将事件与业务数据
-- 写入同一个 PG 事务，后台 publisher 轮询发布到 Redis Stream，保证 at-least-once 语义。
CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    processed_at BIGINT DEFAULT NULL
);

CREATE INDEX idx_outbox_unprocessed ON outbox_events(id) WHERE processed_at IS NULL;
