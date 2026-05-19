import { TeamChip } from './TeamChip';
import type { WinnerPick } from '../types/api';

interface WinnerPickStripProps {
  // The participant's winner-pick history, oldest first. Only the most recent
  // entry is shown; earlier picks were superseded and are no longer displayed.
  picks: WinnerPick[];
}

// WinnerPickStrip shows a participant's current tournament-winner pick on the
// leaderboard as a single team logo. Earlier picks in the history are not
// rendered. "no pick" covers a participant who has never picked a winner.
export function WinnerPickStrip({ picks }: WinnerPickStripProps) {
  if (picks.length === 0) {
    return <span className="text-xs text-zinc-600">no pick</span>;
  }

  const current = picks[picks.length - 1];
  return <TeamChip teamName={current.team_name} />;
}
