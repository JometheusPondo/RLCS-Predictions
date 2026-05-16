-- 004_sheet_source_fields.sql
-- Adds the per-match fields the SheetSource needs: placeholder display text
-- for unresolved bracket slots, and the intra-day slot string (e.g. "2A").
--
-- All three columns are nullable so existing Liquipedia-sourced rows keep
-- their meaning (placeholder_a/b NULL = real team, slot NULL = no slot info).
--
-- Constraint choice: team_a / team_b STAY NOT NULL. For a placeholder bracket
-- row, the SheetSource writes empty strings into team_a / team_b and puts the
-- display text ("Group A First", "Winner of C") in placeholder_a /
-- placeholder_b. ListTeamNames excludes those rows so placeholder strings
-- never bleed into the winner-pick dropdown.

ALTER TABLE matches ADD COLUMN placeholder_a TEXT;
ALTER TABLE matches ADD COLUMN placeholder_b TEXT;
ALTER TABLE matches ADD COLUMN slot          TEXT;
