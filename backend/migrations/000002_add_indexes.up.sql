-- Enterprise rationale: Missing indexes cause full table scans at scale.
-- game_results.user_id: Query user game history (common dashboard query).
-- lobby_states.updated_at: Cleanup loop queries by updated_at to find stale rooms.
-- game_sessions.status: Filter sessions by status (active/ended).
-- Note: These are single-column indexes only. Composite indexes are in migration 000004.
-- Without these, 100x traffic growth makes these queries O(n) instead of O(log n).

CREATE INDEX IF NOT EXISTS idx_game_results_user_id ON game_results(user_id);
CREATE INDEX IF NOT EXISTS idx_lobby_states_updated_at ON lobby_states(updated_at);
CREATE INDEX IF NOT EXISTS idx_game_sessions_status ON game_sessions(status);
