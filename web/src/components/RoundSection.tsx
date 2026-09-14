import type { Match, Pick } from '../types/api';
import type { RoundGroup } from '../lib/matches';
import { MatchCard } from './MatchCard';
import { ReadOnlyMatchCard } from './ReadOnlyMatchCard';
import { useEvent } from '../lib/events';

interface RoundSectionProps {
  group: RoundGroup;
  // pickForMatch returns the participant's current pick for a match, or null.
  pickForMatch: (matchId: string) => Pick | null;
  // onPick bubbles a tap up to the page, which owns the mutation. Ignored when
  // readOnly is true.
  onPick: (matchId: string, side: Pick) => void;
  // readOnly renders non-interactive cards — used when viewing someone else's
  // profile (you can see their completed-match picks but not change anything).
  readOnly?: boolean;
  // bypassLock is forwarded to MatchCard: when true, upcoming/live matches stay
  // tappable even after they lock. Used for the lock-exempt accounts (The Coin,
  // Chat). No effect when readOnly is true.
  bypassLock?: boolean;
  onViewPicks?: (match: Match) => void;
}

// RoundSection renders one round-header section: the round name followed by its
// match cards. Interactive (MatchCard) on your own profile; read-only
// (ReadOnlyMatchCard) when viewing someone else's.
export function RoundSection({
  group,
  pickForMatch,
  onPick,
  readOnly = false,
  bypassLock = false,
  onViewPicks,
}: RoundSectionProps) {
  const { event } = useEvent();
  const timeFormat = new Intl.DateTimeFormat('en-US', {
    hour: 'numeric', minute: '2-digit', timeZone: event.timezone, timeZoneName: 'short',
  });
  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-400">
        {group.round.name}
      </h3>
      <div className="space-y-2">
        {group.matches.map((match) => (
          <div key={match.id} className="space-y-1">
            <p className="flex justify-between px-1 text-xs text-zinc-500">
              <span>{match.scheduled_at && !match.scheduled_at.endsWith('T00:00:00Z') ? timeFormat.format(new Date(match.scheduled_at)) : 'Time to be confirmed'} · Best of {match.best_of}</span>
              <span>{match.status === 'completed' ? 'Final' : match.status === 'live' ? 'Live' : match.locked && !bypassLock ? 'Picks locked' : ''}</span>
            </p>
          {match.locked && onViewPicks && (!bypassLock || match.status === 'completed') ? (
            <button
              type="button"
              onClick={() => onViewPicks(match)}
              aria-label={`View predictions for ${match.team_a || match.placeholder_a || 'Team to be confirmed'} vs ${match.team_b || match.placeholder_b || 'Team to be confirmed'}`}
              aria-haspopup="dialog"
              className="block w-full rounded-lg text-left hover:brightness-110 focus-visible:outline-2 focus-visible:outline-blue-500"
            >
              <ReadOnlyMatchCard match={match} userPick={pickForMatch(match.id)} />
              <span className="block px-1 pt-1 text-xs text-blue-400">View predictions →</span>
            </button>
          ) : readOnly ? (
            <ReadOnlyMatchCard
              key={match.id}
              match={match}
              userPick={pickForMatch(match.id)}
            />
          ) : (
            <MatchCard
              key={match.id}
              match={match}
              userPick={pickForMatch(match.id)}
              onPick={(side) => onPick(match.id, side)}
              bypassLock={bypassLock}
            />
          )}
          {match.locked && onViewPicks && bypassLock && match.status !== 'completed' && (
            <button type="button" onClick={() => onViewPicks(match)} aria-haspopup="dialog" className="px-1 py-1 text-xs text-blue-400 underline underline-offset-2">
              View predictions →
            </button>
          )}
          </div>
        ))}
      </div>
    </section>
  );
}
