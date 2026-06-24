-- 000008_drop_redundant_indexes.down.sql
-- 回滚：重建被删除的冗余索引（仅在需要回滚时使用）。
-- 注意：这些索引是冗余的，重建后应尽快再次删除并重新评估查询计划。

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_sessions_lobby ON game_sessions(lobby_code);
CREATE INDEX IF NOT EXISTS idx_results_session ON game_results(session_id);
CREATE INDEX IF NOT EXISTS idx_lobby_states_updated_at ON lobby_states(updated_at DESC);
