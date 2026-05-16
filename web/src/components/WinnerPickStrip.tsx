import { TeamChip } from './TeamChip';
import type { WinnerPick } from '../types/api';

interface WinnerPickStripProps {
  picks: WinnerPick[];
}

// WinnerPickStrip renders a participant's winner-pick history as a strip of
// team logos. Every pick except the most recent is struck through with a red
// line (it was superseded). The strip is right-aligned and wraps every 3 logos
// onto a new line below — max-w-24 fits exactly three 28px chips plus gaps, so
// the 4th wraps. Because the leaderboard row is items-start, a wrapped strip
// grows downward without stretching the score next to it.
export function WinnerPickStrip({ picks }: WinnerPickStripProps) {
  if (picks.length === 0) {
    return <span className="text-xs text-zinc-600">no pick</span>;
  }

  const lastIndex = picks.length - 1;

  return (
    <div className="flex max-w-24 flex-wrap justify-end gap-1">
      {picks.map((pick, i) => (
        <TeamChip
          key={`${pick.team_name}-${pick.picked_at}`}
          teamName={pick.team_name}
          struck={i !== lastIndex}
        />
      ))}
    </div>
  );
}
