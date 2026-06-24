-- 000008_drop_redundant_indexes.up.sql
-- 企业为何需要：冗余索引拖累写入路径。每个多余索引让 INSERT/UPDATE/DELETE
-- 多一次 B-tree 维护，高写入场景下导致 WAL 膨胀、autovacuum 触发频繁、
-- 缓冲池被冷索引挤占热数据。
--
-- 删除以下冗余索引（被复合索引或 UNIQUE 约束覆盖）：
-- 1. idx_users_email — 与 users.email UNIQUE 约束自动建的索引重复
-- 2. idx_sessions_lobby — 被 idx_game_sessions_lobby_status(lobby_code,status) 覆盖
-- 3. idx_results_session — 被 idx_game_results_session_user(session_id,user_id) 覆盖
-- 4. idx_lobby_states_updated_at — 被 idx_lobby_states_updated_code(updated_at DESC,code) 覆盖

DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_sessions_lobby;
DROP INDEX IF EXISTS idx_results_session;
DROP INDEX IF EXISTS idx_lobby_states_updated_at;
