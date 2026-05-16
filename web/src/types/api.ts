// TypeScript types matching the Go response shapes from internal/models.
// Keep these in sync by hand — there's no codegen for this small surface area.
// When the backend adds/changes fields, update both this file and the Go struct
// JSON tags in the same commit.

export type Pick = 'A' | 'B';

export type MatchStatus = 'upcoming' | 'live' | 'completed';

// Group stage is a single round robin (4 groups of 4), not Swiss.
export type Stage = 'group' | 'bracket';

export interface Participant {
  id: string;
  display_name: string;
  score: number;
  // Append-only tournament-winner pick history, oldest first. The last element
  // is the current pick. Empty when the participant has never picked a winner.
  winner_picks: WinnerPick[];
}

// One entry in a participant's tournament-winner pick history.
export interface WinnerPick {
  team_name: string;
  picked_at: string; // RFC3339
}

// Returned by POST /api/login. The token is the participant id.
export interface LoginResponse {
  token: string;
}

export interface Prediction {
  match_id: string;
  pick: Pick;
}

export interface ParticipantWithPredictions extends Participant {
  predictions: Prediction[];
}

export interface Round {
  // Round.id is intentionally absent — the Go model uses `json:"-"`.
  stage: Stage;
  sort_order: number;
  name: string;
}

export interface Match {
  id: string;
  round: Round;
  team_a: string;
  team_b: string;
  team_a_score: number | null;
  team_b_score: number | null;
  winner: Pick | null;
  status: MatchStatus;
  scheduled_at: string | null; // RFC3339, or null when Liquipedia hasn't published a time
}

export interface SyncStatus {
  last_synced_at: string | null;
  last_error: string | null;
}

// Error envelope returned by every 4xx/5xx. See internal/api/handlers.go.
export interface ApiErrorBody {
  error: string;
  code: string;
}
