-- Audit log table with tamper-proof chain (HMAC-SHA256)
-- 企业为何需要：审计日志必须防篡改以提供不可否认性（non-repudiation），满足 SOC2/ISO27001 合规要求。
-- HMAC 链式哈希：每条记录的 this_hash = HMAC(secret, prev_hash || payload)，篡改任何记录会使后续所有 hash 验证失败。
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    action VARCHAR(100) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    actor_ip VARCHAR(45),
    resource VARCHAR(500),
    before JSONB,
    after JSONB,
    request_id VARCHAR(255),
    trace_id VARCHAR(255),
    prev_hash VARCHAR(64) NOT NULL DEFAULT '',
    this_hash VARCHAR(64) NOT NULL,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

-- Index for time-range queries
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
-- Index for actor queries (e.g., "show all actions by user X")
CREATE INDEX idx_audit_logs_actor_id ON audit_logs(actor_id);
-- Index for action type queries
CREATE INDEX idx_audit_logs_action ON audit_logs(action);

-- Trigger: prevent UPDATE and DELETE on audit_logs (immutability)
-- 企业为何需要：审计日志一旦写入必须不可变。触发器在数据库层强制不可变性，
-- 即使应用层被绕过（如直连 DB 执行 SQL）也无法篡改历史记录。
CREATE OR REPLACE FUNCTION prevent_audit_log_modification() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is immutable: UPDATE and DELETE are not allowed';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER no_update_audit_logs
    BEFORE UPDATE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_log_modification();

CREATE TRIGGER no_delete_audit_logs
    BEFORE DELETE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_log_modification();

CREATE TRIGGER no_truncate_audit_logs
    BEFORE TRUNCATE ON audit_logs
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_audit_log_modification();
