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
  // RFC3339. The real published start time when the broadcast schedule has
  // one; otherwise the match's day at 00:00:00Z (date known, start time not
  // yet published). Null only if the source announced no date at all.
  scheduled_at: string | null;
  // Computed server-side: true when predictions on this match are locked
  // (the day's lock time has passed, the match has started on the final day,
  // or the match is completed). The server enforces this independently; the
  // UI uses it to gate tappability.
  locked: boolean;
  // Computed server-side: which side ('A' | 'B') is the underdog pick — the
  // team fewer humans picked — or null when there's no single underdog side.
  // Revealed only once the match is locked (pick distribution frozen); always
  // null while predictions can still change. See scoring.UnderdogSide.
  underdog: Pick | null;
}

export interface SyncStatus {
  last_synced_at: string | null;
  last_error: string | null;
}

// One participant's projected day swing, from GET /api/simulation. Both values
// are >= 0: best_case = positions that could be GAINED if all of that player's
// picks for the day hit, worst_case = positions that could be LOST if all
// miss. A 0 means they're capped (top/bottom of the board, or a pure-consensus
// slate). See internal/simulation.
export interface SimulationResult {
  participant_id: string;
  best_case: number;
  worst_case: number;
}

// GET /api/simulation response.
export interface SimulationResponse {
  // The day the projection covers (YYYY-MM-DD), or null when the tournament
  // has no unfinished matches left to simulate.
  simulation_day: string | null;
  results: SimulationResult[];
}

// Error envelope returned by every 4xx/5xx. See internal/api/handlers.go.
export interface ApiErrorBody {
  error: string;
  code: string;
}
