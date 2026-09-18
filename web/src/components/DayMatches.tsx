import { useId, useState } from 'react';
import { groupMatchesByDay } from '../lib/matches';
import type { Match, Pick } from '../types/api';
import { RoundSection } from './RoundSection';

interface DayMatchesProps {
  matches: Match[];
  pickForMatch: (id: string) => Pick | null;
  onPick?: (id: string, side: Pick) => void;
  readOnly?: boolean;
  bypassLock?: boolean;
  ownerOverride?: boolean;
  onViewPicks?: (match: Match) => void;
}

export function DayMatches({ matches, pickForMatch, onPick, readOnly = true, bypassLock = false, ownerOverride = false, onViewPicks }: DayMatchesProps) {
  const [expandedDays, setExpandedDays] = useState<Record<string, boolean>>({});
  const sectionId = useId();

  return (
    <div className="space-y-6">
      {groupMatchesByDay(matches).map((day) => {
        const dayMatches = day.rounds.flatMap((group) => group.matches);
        const completed = dayMatches.filter((match) => match.status === 'completed').length;
        const expanded = expandedDays[day.date] ?? completed < dayMatches.length;
        const contentId = `${sectionId}-${day.date || 'unscheduled'}`;

        return (
          <section key={day.date}>
            <h2>
              <button
                type="button"
                aria-expanded={expanded}
                aria-controls={contentId}
                onClick={() => setExpandedDays((previous) => ({ ...previous, [day.date]: !expanded }))}
                className="flex w-full items-center justify-between gap-3 border-b border-zinc-700 py-3 text-left text-zinc-100 hover:text-blue-300 focus-visible:outline-2 focus-visible:outline-blue-500"
              >
                <span className="text-xl font-semibold tracking-tight">{day.title}</span>
                <span className="flex shrink-0 items-center gap-3">
                  <span className="text-xs text-zinc-400">{completed}/{dayMatches.length} complete</span>
                  <span aria-hidden="true" className="text-lg">{expanded ? '−' : '+'}</span>
                </span>
              </button>
            </h2>
            <div id={contentId} hidden={!expanded} className="space-y-5 pt-5">
              {day.rounds.map((group) => (
                <RoundSection key={group.round.name} group={group} pickForMatch={pickForMatch} onPick={(id, side) => onPick?.(id, side)} readOnly={readOnly} bypassLock={bypassLock} ownerOverride={ownerOverride} onViewPicks={onViewPicks} />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}
