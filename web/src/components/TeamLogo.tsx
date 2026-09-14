import { useState } from 'react';

import { logoSrc } from '../lib/logos';

interface TeamLogoProps {
  teamName: string;
  className?: string;
}

// TeamLogo renders just a team's logo image from /logos/<team name>.png. On
// load failure it renders nothing — unlike TeamChip, there's no text fallback,
// because TeamLogo is used alongside the team name text (e.g. on match cards),
// so a missing logo should simply collapse rather than duplicate the name.
export function TeamLogo({ teamName, className }: TeamLogoProps) {
  const [failedName, setFailedName] = useState<string | null>(null);
  const unresolved = !teamName || teamName === 'Team to be confirmed' || /^(Winner of|Loser of|Group |GSL Play-In)/i.test(teamName);
  if (unresolved) return null;
  if (failedName === teamName) {
    const initials = teamName.split(/\s+/).map((word) => word[0]).slice(0, 2).join('').toUpperCase();
    return <span aria-hidden className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-zinc-700 text-[10px] text-zinc-200">{initials}</span>;
  }

  return (
    <img
      src={logoSrc(teamName)}
      alt=""
      aria-hidden
      onError={() => setFailedName(teamName)}
      className={className ?? 'h-6 w-6 shrink-0 object-contain'}
    />
  );
}
