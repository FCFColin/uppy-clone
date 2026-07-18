-- Convert D1 SQLite schema to PostgreSQL
CREATE TABLE users (
  id UUID PRIMARY KEY,
  email VARCHAR(255) UNIQUE NOT NULL,
  nickname VARCHAR(50),
  palette INTEGER DEFAULT 0,
  created_at BIGINT DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000),
  last_login BIGINT
);

CREATE TABLE game_sessions (
  id UUID PRIMARY KEY,
  lobby_code VARCHAR(10) NOT NULL,
  created_by UUID REFERENCES users(id),
  status VARCHAR(20) DEFAULT 'active',
  started_at BIGINT,
  ended_at BIGINT,
  final_score INTEGER DEFAULT 0
);

CREATE TABLE game_results (
  id UUID PRIMARY KEY,
  session_id UUID REFERENCES game_sessions(id),
  user_id UUID REFERENCES users(id),
  score_contribution INTEGER DEFAULT 0,
  taps_count INTEGER DEFAULT 0,
  created_at BIGINT DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)
);

-- Redundant single-column indexes (idx_users_email, idx_sessions_lobby, idx_results_session)
-- removed via reverse-fold of 000008 (spec deep-arch-slim-v2 Task 27):
-- covered by UNIQUE constraint / composite indexes in migration 000004.

-- New tables for Durable Objects replacement
CREATE TABLE lobby_states (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(10) UNIQUE NOT NULL,
  state JSONB NOT NULL,
  updated_at BIGINT DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000),
  created_at BIGINT DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)
);

CREATE TABLE admin_config (
  id VARCHAR(50) PRIMARY KEY,
  config JSONB NOT NULL,
  updated_at BIGINT DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)
);