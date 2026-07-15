-- 000008_drop_redundant_indexes.down.sql
-- 回滚：重建被删除的冗余索引（仅在需要回滚时使用）。
-- 注意：这些索引是冗余的，重建后应尽快再次删除并重新评估查询计划。
-- store-027: idx_lobby_states_updated_at 的原始索引方向为 DESC，
-- 与复合索引 idx_lobby_states_updated_code(updated_at DESC, code) 的首列方向一致。
-- 其他索引无方向指定（默认 ASC），与原始创建语句一致。

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_sessions_lobby ON game_sessions(lobby_code);
CREATE INDEX IF NOT EXISTS idx_results_session ON game_results(session_id);
CREATE INDEX IF NOT EXISTS idx_lobby_states_updated_at ON lobby_states(updated_at DESC);
