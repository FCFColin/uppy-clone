-- Enterprise rationale: ON DELETE CASCADE prevents orphaned records when
-- a parent row is deleted. Without it, deleting a user fails if they have
-- game_results, or leaves orphaned records if forced. CASCADE automatically
-- cleans up child records, maintaining referential integrity.
-- Trade-off: Accidental deletion of a parent cascades to children.
-- Mitigation: Use soft deletes for users (add deleted_at column).

-- Drop existing foreign keys and re-add with CASCADE
ALTER TABLE game_results
    DROP CONSTRAINT IF EXISTS game_results_session_id_fkey,
    ADD CONSTRAINT game_results_session_id_fkey
        FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE;

ALTER TABLE game_results
    DROP CONSTRAINT IF EXISTS game_results_user_id_fkey,
    ADD CONSTRAINT game_results_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE game_sessions
    DROP CONSTRAINT IF EXISTS game_sessions_created_by_fkey,
    ADD CONSTRAINT game_sessions_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;

-- Enterprise rationale: CHECK constraints enforce data integrity at the
-- database level, preventing invalid data even if application validation
-- is bypassed (e.g., direct SQL access, bugs). This is defense-in-depth.
-- Trade-off: Slightly slower inserts, but negligible for this scale.

-- Game session status must be 'active' or 'ended'
ALTER TABLE game_sessions
    ADD CONSTRAINT chk_session_status CHECK (status IN ('active', 'ended'));

-- Score contribution and taps count must be non-negative
ALTER TABLE game_results
    ADD CONSTRAINT chk_score_nonneg CHECK (score_contribution >= 0);

ALTER TABLE game_results
    ADD CONSTRAINT chk_taps_nonneg CHECK (taps_count >= 0);

-- Lobby code must be non-empty and within length limit
ALTER TABLE lobby_states
    ADD CONSTRAINT chk_lobby_code_length CHECK (length(code) > 0 AND length(code) <= 10);
