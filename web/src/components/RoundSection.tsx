import type { Pick } from '../types/api';
import type { RoundGroup } from '../lib/matches';
import { MatchCard } from './MatchCard';
import { ReadOnlyMatchCard } from './ReadOnlyMatchCard';

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
}

// RoundSection renders one round-header section: the round name followed by its
// match cards. Interactive (MatchCard) on your own profile; read-only
// (ReadOnlyMatchCard) when viewing someone else's.
export function RoundSection({ group, pickForMatch, onPick, readOnly = false }: RoundSectionProps) {
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-400">
        {group.round.name}
      </h2>
      <div className="space-y-2">
        {group.matches.map((match) =>
          readOnly ? (
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
            />
          ),
        )}
      </div>
    </section>
  );
}
