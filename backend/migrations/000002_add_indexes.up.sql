-- Enterprise rationale: Missing indexes cause full table scans at scale.
-- game_results.user_id: Query user game history (common dashboard query).
-- game_sessions.status: Filter sessions by status (active/ended).
-- Note: These are single-column indexes only. Composite indexes are in migration 000004.
-- Without these, 100x traffic growth makes these queries O(n) instead of O(log n).
-- idx_lobby_states_updated_at removed via reverse-fold of 000008 (spec deep-arch-slim-v2 Task 27):
-- covered by idx_lobby_states_updated_code(updated_at DESC, code) in migration 000004.

CREATE INDEX IF NOT EXISTS idx_game_results_user_id ON game_results(user_id);
CREATE INDEX IF NOT EXISTS idx_game_sessions_status ON game_sessions(status);