import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { api } from '../api/client';
import { groupMatchesByRound } from '../lib/matches';
import { ADMIN_ID, useAuth } from '../lib/auth';
import { Drawer } from '../components/Drawer';
import { ReadOnlyMatchCard } from '../components/ReadOnlyMatchCard';
import { SkeletonRow } from '../components/Skeleton';
import { WinnerPickStrip } from '../components/WinnerPickStrip';
import type { Match, Participant, Pick } from '../types/api';

// Leaderboard (spec § 7.3): participants sorted by score DESC, click a row to
// open a read-only drawer showing that person's picks across every round.
export function LeaderboardPage() {
  const participantsQuery = useQuery({
    queryKey: ['participants'],
    queryFn: api.getParticipants,
  });

  // Matches are fetched once for the drawer's round grouping and for the
  // "x/y correct" denominator (count of completed matches).
  const matchesQuery = useQuery({
    queryKey: ['matches'],
    queryFn: api.getMatches,
  });

  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Sorted copy — score DESC, then display_name ASC for a stable tiebreak.
  // The blast_admin backstage account is filtered out here: it's a real
  // participant (and shows up in the landing dropdown so the operator can log
  // in), but it must never appear on the leaderboard.
  const ranked = useMemo(() => {
    const list = participantsQuery.data
      ? participantsQuery.data.filter((p) => p.id !== ADMIN_ID)
      : [];
    list.sort((a, b) => b.score - a.score || a.display_name.localeCompare(b.display_name));
    return list;
  }, [participantsQuery.data]);

  // Denominator for "x/y correct": how many matches have actually been played.
  const completedCount = useMemo(
    () => matchesQuery.data?.filter((m) => m.status === 'completed').length ?? 0,
    [matchesQuery.data],
  );

  const selected = ranked.find((p) => p.id === selectedId) ?? null;

  return (
    <main className="mx-auto max-w-2xl px-4 py-6">
      <h1 className="text-2xl font-semibold tracking-tight">Leaderboard</h1>

      {participantsQuery.isPending && (
        <ol className="mt-6 space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </ol>
      )}

      {participantsQuery.error && (
        <p className="mt-6 text-sm text-red-400">
          Couldn&rsquo;t load leaderboard: {participantsQuery.error.message}
        </p>
      )}

      {participantsQuery.data && ranked.length === 0 && (
        <p className="mt-6 text-sm text-zinc-500">No participants yet.</p>
      )}

      {ranked.length > 0 && (
        <ol className="mt-6 space-y-1">
          {ranked.map((p, i) => (
            <LeaderboardRow
              key={p.id}
              rank={i + 1}
              participant={p}
              completedCount={completedCount}
              onClick={() => setSelectedId(p.id)}
            />
          ))}
        </ol>
      )}

      <Drawer
        open={selected !== null}
        onClose={() => setSelectedId(null)}
        title={
          selected && (
            <div>
              <h2 className="text-lg font-semibold tracking-tight">
                {selected.display_name}
              </h2>
              <p className="text-sm text-zinc-400">Score: {selected.score}</p>
            </div>
          )
        }
      >
        {selected && (
          <DrawerBody
            participantId={selected.id}
            matches={matchesQuery.data}
            matchesPending={matchesQuery.isPending}
            matchesError={matchesQuery.error}
          />
        )}
      </Drawer>
    </main>
  );
}

interface LeaderboardRowProps {
  rank: number;
  participant: Participant;
  // completedCount is the "y" in "x/y correct" — total matches played so far.
  completedCount: number;
  onClick: () => void;
}

function LeaderboardRow({ rank, participant, completedCount, onClick }: LeaderboardRowProps) {
  // Row layout, left to right: rank + name + score grouped tightly together,
  // then "x/y correct", then the Predicted Winner strip pushed to the far
  // right with ml-auto.
  //
  // items-start (not items-center): when the winner-pick strip wraps to a
  // second row of logos, the row grows downward and everything else stays put
  // at the top instead of re-centering. pt-1 nudges the text down to sit level
  // with the first row of 28px logo chips.
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        className="flex w-full items-start gap-4 rounded-md border border-zinc-800 bg-zinc-900 px-4 py-3 text-left transition-colors duration-150 hover:bg-zinc-800"
      >
        <span className="flex items-center gap-2 pt-1">
          <span className="w-6 text-sm font-semibold text-zinc-500">{rank}</span>
          <span className="text-sm font-medium text-zinc-100">
            {participant.display_name}
          </span>
          <span className="rounded bg-zinc-800 px-2 py-0.5 text-sm font-semibold text-zinc-100">
            {participant.score}
          </span>
        </span>

        <span className="whitespace-nowrap pt-1 text-xs text-zinc-400">
          {participant.score}/{completedCount} correct
        </span>

        <span className="ml-auto flex items-start gap-2">
          <span className="whitespace-nowrap pt-1 text-xs text-zinc-500">
            Predicted Winner:
          </span>
          <WinnerPickStrip picks={participant.winner_picks} />
        </span>
      </button>
    </li>
  );
}

// DrawerBody fetches the selected participant's predictions and renders their
// picks across every round, read-only.
//
// The participant query key includes the viewer's identity (auth). This is
// load-bearing: the server filters predictions by who's asking (you only see
// other people's *completed*-match picks), so two viewers must not share a
// cache entry. Without the viewer in the key, a stale full-pick entry from
// when that participant viewed their own profile would leak their in-progress
// picks to everyone else for the 30s staleTime window.
interface DrawerBodyProps {
  participantId: string;
  matches: Match[] | undefined;
  matchesPending: boolean;
  matchesError: Error | null;
}

function DrawerBody({ participantId, matches, matchesPending, matchesError }: DrawerBodyProps) {
  const auth = useAuth();

  const participantQuery = useQuery({
    queryKey: ['participant', participantId, auth],
    queryFn: () => api.getParticipant(participantId),
  });

  const grouped = useMemo(
    () => (matches ? groupMatchesByRound(matches) : []),
    [matches],
  );

  if (matchesPending || participantQuery.isPending) {
    return <p className="text-sm text-zinc-500">Loading picks…</p>;
  }

  if (matchesError || participantQuery.error) {
    const err = matchesError ?? participantQuery.error;
    return (
      <p className="text-sm text-red-400">
        Couldn&rsquo;t load picks: {err?.message}
      </p>
    );
  }

  if (grouped.length === 0) {
    return <p className="text-sm text-zinc-500">No matches yet.</p>;
  }

  const picks = participantQuery.data?.predictions ?? [];
  const pickFor = (matchId: string): Pick | null =>
    picks.find((p) => p.match_id === matchId)?.pick ?? null;

  return (
    <div className="space-y-6">
      {grouped.map((group) => (
        <section key={group.round.name} className="space-y-3">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-400">
            {group.round.name}
          </h3>
          <div className="space-y-2">
            {group.matches.map((match) => (
              <ReadOnlyMatchCard
                key={match.id}
                match={match}
                userPick={pickFor(match.id)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
