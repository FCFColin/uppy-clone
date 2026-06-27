-- Database roles for least privilege access
-- 企业为何需要：应用使用最小权限账户，即使 SQL 注入成功也无法执行 DDL 或管理操作。
-- This script runs at database initialization (docker-entrypoint-initdb.d).
-- Migration 000009 only contains GRANT statements (CREATE ROLE must run outside migration transactions).

CREATE ROLE app_user WITH LOGIN PASSWORD 'change_in_production' NOCREATEDB NOCREATEROLE NOSUPERUSER;
CREATE ROLE migrator WITH LOGIN PASSWORD 'change_in_production' NOCREATEDB NOCREATEROLE NOSUPERUSER;
