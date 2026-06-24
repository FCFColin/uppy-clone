-- Add soft delete columns to users table for GDPR Article 17 compliance
-- 企业为何需要：GDPR 第 17 条要求删除用户 PII。软删除模式允许立即匿名化 PII
-- 同时保留用户行用于引用完整性（如游戏结果的外键），30 天后硬删除。
ALTER TABLE users ADD COLUMN deleted_at BIGINT DEFAULT NULL;
ALTER TABLE users ADD COLUMN email_anonymized BOOLEAN DEFAULT false;

-- Index for cleanup job to find users past retention period
CREATE INDEX idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
