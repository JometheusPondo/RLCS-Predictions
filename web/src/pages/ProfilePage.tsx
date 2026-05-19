import { useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, ApiClientError } from '../api/client';
import { groupMatchesByRound } from '../lib/matches';
import { useAuth, isLockExempt } from '../lib/auth';
import { RoundSection } from '../components/RoundSection';
import { SkeletonSection } from '../components/Skeleton';
import { TeamChip } from '../components/TeamChip';
import type { ParticipantWithPredictions, Pick } from '../types/api';

// PredictionVars drives the mutation: 'set' picks a side, 'clear' deletes the
// prediction (tapping the already-picked side).
type PredictionVars =
  | { matchId: string; action: 'set'; pick: Pick }
  | { matchId: string; action: 'clear' };

export function ProfilePage() {
  const { id } = useParams<{ id: string }>();
  const auth = useAuth();
  const queryClient = useQueryClient();

  // Editable only when you're logged in as the profile you're viewing. Viewing
  // anyone else (or being anonymous) is read-only — and the server only returns
  // their completed-match predictions anyway.
  const canEdit = auth !== null && auth === id;

  // The Coin and Chat are lock-exempt accounts: when the operator is logged in
  // as one of them, its match cards stay tappable even after the day locks, so
  // their picks can be entered or changed at any time. The server waives the
  // lock for these accounts in parallel (see lockExemptParticipants in
  // internal/db/queries.go), so the taps actually save. Completed matches stay
  // locked even here — a post-result correction is a rare operator job.
  const bypassLock = canEdit && isLockExempt(id ?? null);

  // The participant query key includes the viewer's identity (auth). The server
  // filters predictions by who's asking — you only see other people's
  // completed-match picks — so two different viewers must not share one cache
  // entry, or a stale full-pick entry would leak in-progress picks across
  // identities for the 30s staleTime window.
  const participantKey = ['participant', id, auth] as const;

  // Matches poll every 30s so new rounds and completed results appear without
  // a manual refresh (spec § 7.2).
  const matchesQuery = useQuery({
    queryKey: ['matches'],
    queryFn: api.getMatches,
    refetchInterval: 30_000,
  });

  // The participant query also polls at 30s so the header score stays current
  // when a match completes (the score is computed server-side on read).
  const participantQuery = useQuery({
    queryKey: participantKey,
    queryFn: () => api.getParticipant(id!),
    enabled: Boolean(id),
    refetchInterval: 30_000,
  });

  // ----- prediction mutation (own profile only) -----
  const mutation = useMutation({
    mutationFn: async (vars: PredictionVars) => {
      if (vars.action === 'set') {
        return api.setPrediction(id!, vars.matchId, vars.pick);
      }
      await api.deletePrediction(id!, vars.matchId);
      return null;
    },
    onMutate: async (vars) => {
      await queryClient.cancelQueries({ queryKey: participantKey });
      const previous = queryClient.getQueryData<ParticipantWithPredictions>(participantKey);
      queryClient.setQueryData<ParticipantWithPredictions>(participantKey, (old) => {
        if (!old) return old;
        const predictions = old.predictions.filter((p) => p.match_id !== vars.matchId);
        if (vars.action === 'set') {
          predictions.push({ match_id: vars.matchId, pick: vars.pick });
        }
        return { ...old, predictions };
      });
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(participantKey, context.previous);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: participantKey });
    },
  });

  // ----- winner pick mutation (own profile only) -----
  const [winnerSelection, setWinnerSelection] = useState('');
  const winnerMutation = useMutation({
    mutationFn: (teamName: string) => api.setWinnerPick(id!, teamName),
    onSuccess: (updated) => {
      queryClient.setQueryData(participantKey, updated);
      // The leaderboard shows winner picks, so refresh that list too.
      void queryClient.invalidateQueries({ queryKey: ['participants'] });
      setWinnerSelection('');
    },
  });

  const grouped = useMemo(
    () => (matchesQuery.data ? groupMatchesByRound(matchesQuery.data) : []),
    [matchesQuery.data],
  );

  // Distinct competing teams, alphabetical — populates the winner-pick dropdown.
  // Derived from matches; no dedicated teams endpoint needed.
  const teams = useMemo(() => {
    if (!matchesQuery.data) return [];
    const set = new Set<string>();
    for (const m of matchesQuery.data) {
      set.add(m.team_a);
      set.add(m.team_b);
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  }, [matchesQuery.data]);

  const pickForMatch = (matchId: string): Pick | null => {
    const pred = participantQuery.data?.predictions.find((p) => p.match_id === matchId);
    return pred ? pred.pick : null;
  };

  const handlePick = (matchId: string, side: Pick) => {
    if (!canEdit) return;
    const current = pickForMatch(matchId);
    if (current === side) {
      mutation.mutate({ matchId, action: 'clear' });
    } else {
      mutation.mutate({ matchId, action: 'set', pick: side });
    }
  };

  if (!id) {
    return <Message>Invalid profile URL.</Message>;
  }

  if (
    participantQuery.error instanceof ApiClientError &&
    participantQuery.error.code === 'not_found'
  ) {
    return <Message>No participant with id &ldquo;{id}&rdquo;.</Message>;
  }

  const winnerPicks = participantQuery.data?.winner_picks ?? [];
  const currentWinnerPick =
    winnerPicks.length > 0 ? winnerPicks[winnerPicks.length - 1] : null;

  // Winner picks lock permanently once Day 1 begins — i.e. once any match has
  // locked. Derived from the match list the page already fetches; no API
  // change needed. The Coin and Chat stay editable, mirroring the prediction-
  // lock exemption and the server (see AddWinnerPick).
  const winnerPickLocked =
    (matchesQuery.data ?? []).some((m) => m.locked) && !isLockExempt(id ?? null);

  return (
    <main className="mx-auto max-w-3xl px-4 py-6 space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">
          {participantQuery.data?.display_name ?? 'Loading\u2026'}
        </h1>
        <p className="mt-1 text-sm text-zinc-400">
          Score: {participantQuery.data?.score ?? '\u2014'}
        </p>
      </header>

      {/* Tournament winner pick — an editable selector on your own profile
          until Day 1 begins; a read-only line otherwise (someone else's
          profile, or your own once winner picks have locked). */}
      {canEdit && !winnerPickLocked ? (
        <section className="space-y-3 rounded-lg border border-zinc-800 bg-zinc-900 p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-400">
              Tournament Winner Pick
            </h2>
            {currentWinnerPick && (
              <span className="flex items-center gap-2 text-sm text-zinc-300">
                current:
                <TeamChip teamName={currentWinnerPick.team_name} />
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <select
              value={winnerSelection}
              onChange={(e) => setWinnerSelection(e.target.value)}
              disabled={teams.length === 0 || winnerMutation.isPending}
              className="flex-1 rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 disabled:opacity-50"
            >
              <option value="" disabled>
                {teams.length === 0 ? 'No teams yet\u2026' : 'Select a team\u2026'}
              </option>
              {teams.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
            <button
              type="button"
              onClick={() => winnerSelection && winnerMutation.mutate(winnerSelection)}
              disabled={!winnerSelection || winnerMutation.isPending}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {winnerMutation.isPending ? 'Applying\u2026' : 'Apply'}
            </button>
          </div>
          {winnerMutation.isError && (
            <p className="text-sm text-red-400">
              Couldn&rsquo;t save winner pick
              {winnerMutation.error instanceof ApiClientError
                ? `: ${winnerMutation.error.message}`
                : '.'}
            </p>
          )}
        </section>
      ) : (
        currentWinnerPick && (
          <section className="flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900 px-4 py-3 text-sm text-zinc-300">
            <span className="font-semibold uppercase tracking-wide text-zinc-400">
              Winner pick:
            </span>
            <TeamChip teamName={currentWinnerPick.team_name} />
            <span>{currentWinnerPick.team_name}</span>
          </section>
        )
      )}

      {mutation.isError && (
        <div className="rounded-md border border-red-800 bg-red-950 px-3 py-2 text-sm text-red-200">
          Couldn&rsquo;t save that pick
          {mutation.error instanceof ApiClientError ? `: ${mutation.error.message}` : '.'}
        </div>
      )}

      {matchesQuery.isPending && (
        <>
          <SkeletonSection />
          <SkeletonSection />
        </>
      )}

      {matchesQuery.error && (
        <p className="text-sm text-red-400">
          Couldn&rsquo;t load matches: {matchesQuery.error.message}
        </p>
      )}

      {matchesQuery.data && grouped.length === 0 && (
        <p className="text-sm text-zinc-500">No matches yet.</p>
      )}

      {grouped.map((group) => (
        <RoundSection
          key={group.round.name}
          group={group}
          pickForMatch={pickForMatch}
          onPick={handlePick}
          readOnly={!canEdit}
          bypassLock={bypassLock}
        />
      ))}
    </main>
  );
}

// Message is a private full-page centered notice for invalid-URL / not-found.
function Message({ children }: { children: ReactNode }) {
  return (
    <main className="mx-auto max-w-3xl px-4 py-12">
      <p className="text-sm text-zinc-400">{children}</p>
    </main>
  );
}
