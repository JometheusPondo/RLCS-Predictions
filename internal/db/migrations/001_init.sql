-- 001_init.sql
-- Initial schema for the RLCS Predictions site.
-- See spec § 4 for documentation of each table.
-- Booleans are stored as INTEGER (0/1). Timestamps are RFC3339 TEXT.

CREATE TABLE participants (
    id           TEXT PRIMARY KEY,        -- slug derived from display_name (lowercase, alphanumerics + hyphens)
    display_name TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE tournaments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    liquipedia_page TEXT NOT NULL UNIQUE, -- e.g. "Rocket_League_Championship_Series/2026/Paris_Major"
    name            TEXT NOT NULL,
    is_active       INTEGER NOT NULL DEFAULT 1,
    last_synced_at  TEXT
);

CREATE TABLE rounds (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    stage         TEXT NOT NULL,         -- 'group' | 'bracket'
    sort_order    INTEGER NOT NULL,      -- monotonic across stages; group < bracket
    name          TEXT NOT NULL,         -- 'Group Stage - Round 1', 'Quarterfinals', etc.
    UNIQUE (tournament_id, stage, name)
);

CREATE TABLE matches (
    id           TEXT PRIMARY KEY,       -- stable hash from (tournament, stage, round, team_a, team_b sorted); see spec § 5.4
    round_id     INTEGER NOT NULL REFERENCES rounds(id) ON DELETE CASCADE,
    team_a       TEXT NOT NULL,
    team_b       TEXT NOT NULL,
    team_a_score INTEGER,                -- nullable until decided
    team_b_score INTEGER,
    winner       TEXT,                   -- 'A' | 'B' | NULL
    status       TEXT NOT NULL,          -- 'upcoming' | 'live' | 'completed'
    scheduled_at TEXT,
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_matches_round  ON matches(round_id);
CREATE INDEX idx_matches_status ON matches(status);

CREATE TABLE predictions (
    participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    match_id       TEXT NOT NULL REFERENCES matches(id)       ON DELETE CASCADE,
    pick           TEXT NOT NULL,        -- 'A' | 'B'
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (participant_id, match_id)
);

CREATE INDEX idx_predictions_match ON predictions(match_id);
