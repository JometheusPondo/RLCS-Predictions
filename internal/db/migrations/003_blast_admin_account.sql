-- 003_blast_admin_account.sql
-- Ensures the blast_admin backstage account exists.
--
-- Migration 002 was intended to create this row alongside the password column
-- and winner_pick_history table. But where 002 had already been recorded as
-- applied before its INSERT line was finalized, the schema changes are present
-- while the row is missing — so blast_admin never appears in the landing-page
-- dropdown and the operator can't log in.
--
-- This migration re-applies just the INSERT. It's a new filename, so the
-- runner executes it; INSERT OR IGNORE makes it a harmless no-op on databases
-- where 002 already created the row.
INSERT OR IGNORE INTO participants (id, display_name, password)
VALUES ('blast_admin', 'BLAST Admin', 'rlcscopter');
