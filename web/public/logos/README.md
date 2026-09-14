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
  - On match cards: team initials; unresolved slots show no logo.

So partial coverage is fine — add logos as you collect them.

PNG with transparency, roughly square. They render at 24-28px.

Note: this directory is `web/public/logos/`. Vite serves `web/public/` at the
site root, so these files are reachable at `/logos/<name>.png`. Files placed
under `web/src/` are NOT served — they're source code, not static assets.

Worlds additions come directly from the broadcast sheet's Team Information tab:

- [Bigodes](https://drive.google.com/open?id=13nFSOCBCuYSY0VctjTD2OGN2QUCJ8_F-)
- [Virtus.pro](https://drive.google.com/open?id=13W9JAcYGCHYkDgBi9s__tNsf36AQT7RR)
- [Team Falcons](https://drive.google.com/open?id=1qQa30Uv41EijtyZb5WDgdPJqkZ2rwXsy)

Mate y Tapa and the independent side-event entrants have no logo link in that
sheet and retain the initials fallback.
