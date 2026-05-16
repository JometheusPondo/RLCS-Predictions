import type { Match, Pick } from '../types/api';
import { sideState, type SideState, type SideVisual } from '../lib/matches';
import { TeamLogo } from './TeamLogo';

interface MatchCardProps {
  match: Match;
  userPick: Pick | null;
  // onPick is called with the side that was tapped. The parent decides whether
  // that means "set" or "clear" (tapping the already-picked side clears).
  onPick: (side: Pick) => void;
}

// Tailwind classes per SideVisual. Colors are from spec § 7.2; neutral uses a
// subtle dark surface from the zinc scale.
const visualClasses: Record<SideVisual, string> = {
  blue: 'bg-blue-600 text-white',
  green: 'bg-emerald-600 text-white',
  red: 'bg-red-600 text-white',
  neutral: 'bg-zinc-800 text-zinc-100',
  'winner-outline': 'bg-zinc-800 text-zinc-100 ring-1 ring-inset ring-emerald-500/60',
};

export function MatchCard({ match, userPick, onPick }: MatchCardProps) {
  const a = sideState('A', match, userPick);
  const b = sideState('B', match, userPick);

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
          onClick={a.tappable ? () => onPick('A') : undefined}
        />
        <div className="flex shrink-0 items-center px-3 text-sm font-medium text-zinc-400">
          {center}
        </div>
        <Side
          teamName={match.team_b}
          state={b}
          align="right"
          onClick={b.tappable ? () => onPick('B') : undefined}
        />
      </div>
    </div>
  );
}

// Side is a private helper — not a reusable component, just the left/right
// half of a MatchCard. Renders as a <button> when tappable, a <div> otherwise.
// Layout puts the logo on the card's outer edge: [logo name] on the left,
// [name logo] on the right.
interface SideProps {
  teamName: string;
  state: SideState;
  align: 'left' | 'right';
  onClick?: () => void;
}

function Side({ teamName, state, align, onClick }: SideProps) {
  const className = [
    'flex flex-1 items-center gap-2 px-4 py-3 text-sm font-medium transition-colors duration-150',
    visualClasses[state.visual],
    align === 'left' ? 'justify-start text-left' : 'justify-end text-right',
    onClick ? 'cursor-pointer hover:brightness-110' : 'cursor-default',
  ].join(' ');

  const content =
    align === 'left' ? (
      <>
        <TeamLogo teamName={teamName} />
        <span>{teamName}</span>
      </>
    ) : (
      <>
        <span>{teamName}</span>
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
