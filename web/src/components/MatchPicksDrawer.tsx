import { useQuery } from '@tanstack/react-query';

import { api } from '../api/client';
import { useEvent } from '../lib/events';
import { isLockExempt } from '../lib/auth';
import type { Match, Pick } from '../types/api';
import { Drawer } from './Drawer';
import { TeamLogo } from './TeamLogo';

interface MatchPicksDrawerProps {
  match: Match | null;
  onClose: () => void;
}

// MatchPicksDrawer loads the selected locked match and groups its participants by side.
export function MatchPicksDrawer({ match, onClose }: MatchPicksDrawerProps) {
  const { event } = useEvent();
  const open = match !== null && match.locked;
  const picksQuery = useQuery({
    queryKey: ['match-pickers', event.id, match?.id],
    queryFn: () => api.getMatchPickers(match!.id, event.id),
    enabled: open,
    staleTime: 0,
    refetchInterval: open && event.is_active ? 30_000 : false,
    retry: false,
  });

  return (
    <Drawer open={open} onClose={onClose} title={<h2 className="text-lg font-semibold">Match predictions</h2>}>
      {open && (
        <>
          <p className="mb-4 text-xs text-zinc-400">{match.round.name}</p>
          {picksQuery.isPending && <p role="status" className="text-sm text-zinc-400">Loading predictions…</p>}
          {picksQuery.isError && <p role="alert" className="text-sm text-red-400">Couldn’t load predictions: {picksQuery.error.message}</p>}
          {picksQuery.isSuccess && (
            <div className="space-y-4">
              {(['A', 'B'] as Pick[]).map((side) => {
                const team = side === 'A'
                  ? match.team_a || match.placeholder_a || 'Team to be confirmed'
                  : match.team_b || match.placeholder_b || 'Team to be confirmed';
                const pickers = picksQuery.data.filter((picker) => picker.pick === side);

                return (
                  <section key={side} className="min-w-0 rounded-lg border border-zinc-800 bg-zinc-900">
                    <header className="space-y-2 border-b border-zinc-800 p-3">
                      <TeamLogo teamName={team} className="h-8 w-8 object-contain" />
                      <h3 className="break-words text-sm font-semibold">{team}</h3>
                      <p className="text-xs text-zinc-400">{pickers.length} {pickers.length === 1 ? 'pick' : 'picks'}</p>
                    </header>
                    {pickers.length === 0 ? (
                      <p className="p-3 text-sm text-zinc-500">No picks</p>
                    ) : (
                      <ul className="divide-y divide-zinc-800">
                        {pickers.map((picker) => (
                          <li key={picker.id} className="break-words px-3 py-2 text-sm">
                            {picker.display_name}
                            {isLockExempt(picker.id) && <span className="mt-1 block text-xs text-zinc-500">Benchmark</span>}
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                );
              })}
            </div>
          )}
          <p className="mt-4 text-xs text-zinc-500">Coin and Chat are included here but do not count toward the underdog total.</p>
        </>
      )}
    </Drawer>
  );
}
