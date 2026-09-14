import { groupMatchesByDay } from '../lib/matches';
import type { Match, Pick } from '../types/api';
import { RoundSection } from './RoundSection';

interface DayMatchesProps {
  matches: Match[];
  pickForMatch: (id: string) => Pick | null;
  onPick?: (id: string, side: Pick) => void;
  readOnly?: boolean;
  bypassLock?: boolean;
  onViewPicks?: (match: Match) => void;
}

export function DayMatches({ matches, pickForMatch, onPick, readOnly = true, bypassLock = false, onViewPicks }: DayMatchesProps) {
  return (
    <div className="space-y-10">
      {groupMatchesByDay(matches).map((day) => (
        <section key={day.date} className="space-y-5">
          <h2 className="border-b border-zinc-700 pb-3 text-xl font-semibold tracking-tight text-zinc-100">{day.title}</h2>
          {day.rounds.map((group) => (
            <RoundSection key={group.round.name} group={group} pickForMatch={pickForMatch} onPick={(id, side) => onPick?.(id, side)} readOnly={readOnly} bypassLock={bypassLock} onViewPicks={onViewPicks} />
          ))}
        </section>
      ))}
    </div>
  );
}
