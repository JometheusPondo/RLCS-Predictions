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
  const [failed, setFailed] = useState(false);
  if (failed) return null;

  return (
    <img
      src={logoSrc(teamName)}
      alt=""
      aria-hidden
      onError={() => setFailed(true)}
      className={className ?? 'h-6 w-6 shrink-0 object-contain'}
    />
  );
}
