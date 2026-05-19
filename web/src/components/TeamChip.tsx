import { useState } from 'react';

import { logoSrc } from '../lib/logos';

interface TeamChipProps {
  teamName: string;
}

// TeamChip renders a team's logo from /logos/<team name>.png. If that file
// doesn't exist yet, the <img> onError handler swaps in a text chip — so logos
// can be added incrementally to web/public/logos/ with no code change and no
// manifest. Used where there's no other label for the team (the winner pick
// on the leaderboard, the current-pick line on the profile page).
export function TeamChip({ teamName }: TeamChipProps) {
  const [imgFailed, setImgFailed] = useState(false);

  return (
    <span className="inline-flex items-center" title={teamName}>
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
    </span>
  );
}
