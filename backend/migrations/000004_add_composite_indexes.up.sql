-- 企业为何需要：复合索引遵循最左前缀原则，让 JOIN 查询从全表扫描变为索引覆盖。
-- 单列索引无法优化多列 WHERE/JOIN 条件，复合索引可以。
CREATE INDEX IF NOT EXISTS idx_game_results_session_user ON game_results(session_id, user_id);
CREATE INDEX IF NOT EXISTS idx_game_sessions_lobby_status ON game_sessions(lobby_code, status);
CREATE INDEX IF NOT EXISTS idx_lobby_states_updated_code ON lobby_states(updated_at DESC, code);
