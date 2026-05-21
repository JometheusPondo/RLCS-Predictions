import type { Match, Pick } from '../types/api';
import { sideState, sideRingClass, type SideVisual } from '../lib/matches';
import { TeamLogo } from './TeamLogo';
import { UnderdogBadge } from './UnderdogBadge';

interface ReadOnlyMatchCardProps {
  match: Match;
  userPick: Pick | null;
}

// Same Tailwind mapping as MatchCard. Kept as a separate const (rather than
// shared/exported) so the two card variants can drift independently if needed.
// Rings — the winner outline and the underdog ring — come from sideRingClass.
const visualClasses: Record<SideVisual, string> = {
  blue: 'bg-blue-600 text-white',
  green: 'bg-emerald-600 text-white',
  red: 'bg-red-600 text-white',
  neutral: 'bg-zinc-800 text-zinc-100',
  'winner-outline': 'bg-zinc-800 text-zinc-100',
};

// ReadOnlyMatchCard renders a match with the participant's pick highlighted
// using the § 7.2 color rules, but with no click handlers and no hover
// affordance — used in the leaderboard drawer (spec § 7.3) and for other
// users' profiles. Logos sit on the card's outer edges: [logo name] on the
// left, [name logo] on the right. The underdog team (Match.underdog, set
// only on locked matches) gets an orange ring; the "Underdog: X picks"
// label sits in a footer row below the sides.
export function ReadOnlyMatchCard({ match, userPick }: ReadOnlyMatchCardProps) {
  const a = sideState('A', match, userPick);
  const b = sideState('B', match, userPick);

  const center =
    match.team_a_score !== null && match.team_b_score !== null
      ? `${match.team_a_score} \u2014 ${match.team_b_score}`
      : 'vs';

  // "no prediction" label for upcoming/live matches the participant skipped.
  const noPick = userPick === null && match.status !== 'completed';

  const aUnderdog = match.underdog?.side === 'A';
  const bUnderdog = match.underdog?.side === 'B';

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      <div className="flex items-stretch">
        <div
          className={`flex min-w-0 flex-1 items-center justify-start gap-2 px-4 py-3 text-left text-sm font-medium ${visualClasses[a.visual]} ${sideRingClass(a.visual, aUnderdog)}`}
        >
          <TeamLogo teamName={match.team_a} />
          <span className="truncate">{match.team_a}</span>
        </div>
        <div className="flex shrink-0 items-center px-3 text-sm font-medium text-zinc-400">
          {center}
        </div>
        <div
          className={`flex min-w-0 flex-1 items-center justify-end gap-2 px-4 py-3 text-right text-sm font-medium ${visualClasses[b.visual]} ${sideRingClass(b.visual, bUnderdog)}`}
        >
          <span className="truncate">{match.team_b}</span>
          <TeamLogo teamName={match.team_b} />
        </div>
      </div>
      {match.underdog && (
        <UnderdogBadge side={match.underdog.side} picks={match.underdog.picks} />
      )}
      {noPick && (
        <div className="bg-zinc-900 px-4 py-1 text-center text-xs text-zinc-500">
          no prediction
        </div>
      )}
    </div>
  );
}
