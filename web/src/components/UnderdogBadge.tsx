import type { Pick } from '../types/api';

interface UnderdogBadgeProps {
  side: Pick;
  picks: number;
}

// UnderdogBadge is the "Underdog: X picks" footer row shown beneath a match
// card's sides, aligned under the underdog team. The backend only sets
// Match.underdog on locked matches, so this never appears while predictions
// can still change.
export function UnderdogBadge({ side, picks }: UnderdogBadgeProps) {
  const align = side === 'B' ? 'text-right' : 'text-left';
  const noun = picks === 1 ? 'pick' : 'picks';
  return (
    <div className={`bg-zinc-900 px-4 py-1 text-xs font-medium text-orange-400 ${align}`}>
      Underdog: {picks} {noun}
    </div>
  );
}
