CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  nickname TEXT,
  palette INTEGER DEFAULT 0,
  created_at INTEGER DEFAULT (unixepoch()),
  last_login INTEGER
);

CREATE TABLE game_sessions (
  id TEXT PRIMARY KEY,
  lobby_code TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  status TEXT DEFAULT 'active',
  started_at INTEGER,
  ended_at INTEGER,
  final_score INTEGER DEFAULT 0
);

CREATE TABLE game_results (
  id TEXT PRIMARY KEY,
  session_id TEXT REFERENCES game_sessions(id),
  user_id TEXT REFERENCES users(id),
  score_contribution INTEGER DEFAULT 0,
  taps_count INTEGER DEFAULT 0,
  created_at INTEGER DEFAULT (unixepoch())
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_sessions_lobby ON game_sessions(lobby_code);
CREATE INDEX idx_results_session ON game_results(session_id);
