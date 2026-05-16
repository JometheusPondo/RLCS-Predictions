// logoSrc maps a team name to its logo URL under the served /logos/ directory.
//
// Logo files are named with the team's exact name as it appears in match data
// (e.g. "Team Vitality.png", "FUT Esports.png"), so the only transform is
// URL-encoding to make spaces and punctuation safe in the path. To add a new
// team's logo, drop "<exact team name>.png" into web/public/logos/ — no code
// change, no slug rules to remember.
export function logoSrc(teamName: string): string {
  return `/logos/${encodeURIComponent(teamName)}.png`;
}
