-- 002_auth_and_winner_picks.sql
-- Adds password auth and per-participant tournament-winner picks.
-- See the conversation spec: plaintext passwords (honor-system tool,
-- hand-assigned), append-only winner-pick history.

-- Plaintext password. Nullable: participants created before this migration
-- (and any created without one) have NULL and cannot authenticate until an
-- operator assigns a password directly in the DB.
ALTER TABLE participants ADD COLUMN password TEXT;

-- Append-only history of tournament-winner picks. The participant's *current*
-- pick is simply the most recent row by picked_at. Earlier rows are retained
-- so the leaderboard can render the struck-through history strip.
CREATE TABLE winner_pick_history (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    team_name      TEXT NOT NULL,
    picked_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_winner_pick_history_participant
    ON winner_pick_history(participant_id, picked_at);

-- blast_admin: a backstage reference account for the graphics operator. It can
-- authenticate and read everything, but is filtered out of the public
-- participant list, the landing dropdown, and the leaderboard. Created here so
-- it exists from first run after this migration.
--
-- INSERT OR IGNORE: harmless if a row with this id somehow already exists.
INSERT OR IGNORE INTO participants (id, display_name, password)
VALUES ('blast_admin', 'BLAST Admin', 'rlcscopter');
