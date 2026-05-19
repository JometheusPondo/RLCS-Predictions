import type { Match, Pick } from '../types/api';
import { sideState, sideRingClass, type SideState, type SideVisual } from '../lib/matches';
import { TeamLogo } from './TeamLogo';
import { UnderdogBadge } from './UnderdogBadge';

interface MatchCardProps {
  match: Match;
  userPick: Pick | null;
  // onPick is called with the side that was tapped. The parent decides whether
  // that means "set" or "clear" (tapping the already-picked side clears).
  onPick: (side: Pick) => void;
  // bypassLock keeps upcoming/live matches tappable even when match.locked is
  // true. Used for the lock-exempt accounts (The Coin, Chat) so the operator
  // can set their picks at any time. Completed matches stay non-interactive.
  bypassLock?: boolean;
}

// Tailwind background/text per SideVisual. Colors are from spec § 7.2; neutral
// uses a subtle dark surface from the zinc scale. Rings — the winner outline
// and the underdog ring — are applied separately via sideRingClass.
const visualClasses: Record<SideVisual, string> = {
  blue: 'bg-blue-600 text-white',
  green: 'bg-emerald-600 text-white',
  red: 'bg-red-600 text-white',
  neutral: 'bg-zinc-800 text-zinc-100',
  'winner-outline': 'bg-zinc-800 text-zinc-100',
};

export function MatchCard({ match, userPick, onPick, bypassLock = false }: MatchCardProps) {
  const a = sideState('A', match, userPick, bypassLock);
  const b = sideState('B', match, userPick, bypassLock);

  // Center shows the score once both are present, otherwise "vs".
  const center =
    match.team_a_score !== null && match.team_b_score !== null
      ? `${match.team_a_score} \u2014 ${match.team_b_score}`
      : 'vs';

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      <div className="flex items-stretch">
        <Side
          teamName={match.team_a}
          state={a}
          align="left"
          isUnderdog={match.underdog === 'A'}
          onClick={a.tappable ? () => onPick('A') : undefined}
        />
        <div className="flex shrink-0 items-center px-3 text-sm font-medium text-zinc-400">
          {center}
        </div>
        <Side
          teamName={match.team_b}
          state={b}
          align="right"
          isUnderdog={match.underdog === 'B'}
          onClick={b.tappable ? () => onPick('B') : undefined}
        />
      </div>
    </div>
  );
}

// Side is a private helper — not a reusable component, just the left/right
// half of a MatchCard. Renders as a <button> when tappable, a <div> otherwise.
// Layout puts the logo on the card's outer edge: [logo name] on the left,
// [name logo] on the right. When this side is the underdog, the "Underdog"
// badge sits on the inner side of the name and an orange ring wraps the box.
interface SideProps {
  teamName: string;
  state: SideState;
  align: 'left' | 'right';
  // isUnderdog draws the orange underdog ring + "Underdog" badge on this side.
  isUnderdog: boolean;
  onClick?: () => void;
}

function Side({ teamName, state, align, isUnderdog, onClick }: SideProps) {
  const className = [
    'flex min-w-0 flex-1 items-center gap-2 px-4 py-3 text-sm font-medium transition-colors duration-150',
    visualClasses[state.visual],
    sideRingClass(state.visual, isUnderdog),
    align === 'left' ? 'justify-start text-left' : 'justify-end text-right',
    onClick ? 'cursor-pointer hover:brightness-110' : 'cursor-default',
  ].join(' ');

  const name = <span className="truncate">{teamName}</span>;
  const content =
    align === 'left' ? (
      <>
        <TeamLogo teamName={teamName} />
        {name}
        {isUnderdog && <UnderdogBadge />}
      </>
    ) : (
      <>
        {isUnderdog && <UnderdogBadge />}
        {name}
        <TeamLogo teamName={teamName} />
      </>
    );

  if (onClick) {
    return (
      <button type="button" onClick={onClick} className={className}>
        {content}
      </button>
    );
  }
  return <div className={className}>{content}</div>;
}
