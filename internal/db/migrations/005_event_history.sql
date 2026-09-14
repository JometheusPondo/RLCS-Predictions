ALTER TABLE tournaments ADD COLUMN timezone TEXT NOT NULL DEFAULT 'Europe/Paris';

CREATE TABLE event_winner_pick_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    team_name TEXT NOT NULL,
    picked_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO event_winner_pick_history (id, tournament_id, participant_id, team_name, picked_at)
SELECT id,
    (SELECT MIN(id) FROM tournaments HAVING COUNT(*) = 1),
    participant_id, team_name, picked_at
FROM winner_pick_history;

DROP TABLE winner_pick_history;
ALTER TABLE event_winner_pick_history RENAME TO winner_pick_history;
CREATE INDEX idx_winner_pick_event ON winner_pick_history(tournament_id, participant_id, picked_at, id);

ALTER TABLE matches ADD COLUMN best_of INTEGER NOT NULL DEFAULT 5;
ALTER TABLE matches ADD COLUMN event_date TEXT;
UPDATE matches SET best_of = 7 WHERE round_id IN (SELECT id FROM rounds WHERE stage = 'bracket');
UPDATE matches SET event_date = substr(scheduled_at, 1, 10) WHERE scheduled_at IS NOT NULL;

UPDATE tournaments SET name = 'Paris Major 2026'
WHERE liquipedia_page = 'Rocket_League_Championship_Series/2026/Paris_Major';
