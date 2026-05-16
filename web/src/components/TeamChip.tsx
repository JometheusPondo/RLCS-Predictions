import { useState } from 'react';

import { logoSrc } from '../lib/logos';

interface TeamChipProps {
  teamName: string;
  // struck: draw a red line through the chip — used on superseded winner picks.
  struck?: boolean;
}

// TeamChip renders a team's logo from /logos/<team name>.png. If that file
// doesn't exist yet, the <img> onError handler swaps in a text chip — so logos
// can be added incrementally to web/public/logos/ with no code change and no
// manifest. Used where there's no other label for the team (the winner-pick
// strip on the leaderboard, the current-pick line on the profile page).
export function TeamChip({ teamName, struck = false }: TeamChipProps) {
  const [imgFailed, setImgFailed] = useState(false);

  return (
    <span className="relative inline-flex items-center" title={teamName}>
      {imgFailed ? (
        <span className="whitespace-nowrap rounded bg-zinc-800 px-2 py-1 text-xs font-medium text-zinc-200">
          {teamName}
        </span>
      ) : (
        <img
          src={logoSrc(teamName)}
          alt={teamName}
          onError={() => setImgFailed(true)}
          className="h-7 w-7 rounded object-contain"
        />
      )}
      {struck && (
        <span
          aria-hidden
          className="pointer-events-none absolute inset-x-0 top-1/2 h-0.5 -translate-y-1/2 bg-red-500"
        />
      )}
    </span>
  );
}
