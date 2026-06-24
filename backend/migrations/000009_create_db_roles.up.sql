-- Database roles for least privilege access
-- 企业为何需要：应用使用最小权限账户，即使 SQL 注入成功也无法执行 DDL 或管理操作。
-- NOTE: CREATE ROLE is in docker/init-scripts/01-create-roles.sql (runs at DB init, outside migration transactions).
-- This migration only contains GRANT statements (CREATE ROLE fails inside golang-migrate transactions on PG 16).

-- App user: can only DML on application tables
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;

-- Allow app_user to use tables created by future migrations
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO app_user;

-- Migrator: can run DDL (migrations only)
GRANT ALL ON SCHEMA public TO migrator;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO migrator;
