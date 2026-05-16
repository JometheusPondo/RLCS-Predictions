# Team logos

Drop team logo files here as `<exact team name>.png` and they'll appear
automatically on the leaderboard and on the match cards — no code change, no
manifest.

The filename must match the team's name exactly as it appears in match data
(the same text shown on the match cards). The app URL-encodes it to build the
path, so spaces and punctuation in the name are fine.

Examples:
  Team Vitality   -> Team Vitality.png
  FUT Esports     -> FUT Esports.png
  Karmine Corp    -> Karmine Corp.png

Any team without a matching file falls back gracefully:
  - On the leaderboard winner-pick strip: a text chip with the team name.
  - On match cards: nothing (the team name text is already shown beside it).

So partial coverage is fine — add logos as you collect them.

PNG with transparency, roughly square. They render at 24-28px.

Note: this directory is `web/public/logos/`. Vite serves `web/public/` at the
site root, so these files are reachable at `/logos/<name>.png`. Files placed
under `web/src/` are NOT served — they're source code, not static assets.
