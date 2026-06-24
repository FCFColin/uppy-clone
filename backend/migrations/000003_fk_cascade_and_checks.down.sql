-- Revert CASCADE back to RESTRICT (default)
ALTER TABLE game_results
    DROP CONSTRAINT IF EXISTS game_results_session_id_fkey,
    ADD CONSTRAINT game_results_session_id_fkey
        FOREIGN KEY (session_id) REFERENCES game_sessions(id);

ALTER TABLE game_results
    DROP CONSTRAINT IF EXISTS game_results_user_id_fkey,
    ADD CONSTRAINT game_results_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE game_sessions
    DROP CONSTRAINT IF EXISTS game_sessions_created_by_fkey,
    ADD CONSTRAINT game_sessions_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id);

-- Drop CHECK constraints
ALTER TABLE game_sessions DROP CONSTRAINT IF EXISTS chk_session_status;
ALTER TABLE game_results DROP CONSTRAINT IF EXISTS chk_score_nonneg;
ALTER TABLE game_results DROP CONSTRAINT IF EXISTS chk_taps_nonneg;
ALTER TABLE lobby_states DROP CONSTRAINT IF EXISTS chk_lobby_code_length;
