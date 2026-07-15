CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_game_sessions_leaderboard
    ON game_sessions (final_score DESC, ended_at ASC)
    WHERE status = 'ended' AND final_score > 0;
