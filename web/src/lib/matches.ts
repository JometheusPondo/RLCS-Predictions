import type { Match, Pick, Round } from '../types/api';

// RoundGroup is one round-header section on the profile/leaderboard pages:
// the round metadata plus the matches that belong to it.
export interface RoundGroup {
  round: Round;
  matches: Match[];
}

// groupMatchesByRound buckets matches by round name, orders the groups by
// round.sort_order (ascending — group stage before bracket), and orders
// matches within each group by scheduled time (unscheduled last), then id
// for a stable tiebreak.
export function groupMatchesByRound(matches: Match[]): RoundGroup[] {
  const groups = new Map<string, RoundGroup>();
  for (const m of matches) {
    const existing = groups.get(m.round.name);
    if (existing) {
      existing.matches.push(m);
    } else {
      groups.set(m.round.name, { round: m.round, matches: [m] });
    }
  }

  const result = [...groups.values()];
  result.sort((a, b) => a.round.sort_order - b.round.sort_order);
  for (const group of result) {
    group.matches.sort(compareMatches);
  }
  return result;
}

function compareMatches(a: Match, b: Match): number {
  if (a.scheduled_at && b.scheduled_at) {
    const cmp = a.scheduled_at.localeCompare(b.scheduled_at);
    if (cmp !== 0) return cmp;
  } else if (a.scheduled_at) {
    return -1; // a scheduled, b not → a first
  } else if (b.scheduled_at) {
    return 1; // b scheduled, a not → b first
  }
  return a.id.localeCompare(b.id);
}

// SideVisual is the visual treatment for one side (A or B) of a match card.
// 'winner-outline' is the subtle ring on the actual winner when the user
// didn't pick that side (spec § 7.2).
export type SideVisual = 'blue' | 'green' | 'red' | 'neutral' | 'winner-outline';

export interface SideState {
  visual: SideVisual;
  tappable: boolean;
}

// sideState computes the visual treatment + interactivity for one side of a
// match card. Direct implementation of the table in spec § 7.2:
//
//   upcoming/live, not picked → neutral, tappable
//   upcoming/live, picked     → blue, tappable (tap again to clear)
//   completed,    picked+won  → green, locked
//   completed,    picked+lost → red, locked
//   completed,    won (unpicked) → winner-outline, locked
//   completed,    other       → neutral, locked
export function sideState(side: Pick, match: Match, userPick: Pick | null): SideState {
  const completed = match.status === 'completed';

  if (!completed) {
    return { visual: userPick === side ? 'blue' : 'neutral', tappable: true };
  }

  const thisSideWon = match.winner === side;
  if (userPick === side) {
    return { visual: thisSideWon ? 'green' : 'red', tappable: false };
  }
  return { visual: thisSideWon ? 'winner-outline' : 'neutral', tappable: false };
}
