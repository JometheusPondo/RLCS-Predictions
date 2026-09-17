# RLCS Predictions

A Go/SQLite server and React/TypeScript frontend for broadcast talent predictions.
World Championship 2026 is the active event. Existing Major data remains in the
same database and is available through the read-only Event selector.

## Worlds 2026

The site imports the public [Worlds broadcast sheet](https://docs.google.com/spreadsheets/d/1BuyYGV59e_fR8fUkdgIRiFheBUaokulpj7pRtE39f-c/edit):

| Tab | GID | Scheduled matches |
| --- | --- | ---: |
| GSL Play-In Output | 1174796019 | 10 |
| Groups Output | 10266191 | 24 |
| Bracket Output | 1847119516 | 13 |
| 2v2 + 1v1 Output | 381597648 | 6 |
| Overall Schedule | 1663218581 | Start times |

All 53 matches count toward one Worlds leaderboard. Play-ins and groups are
best of five; playoffs, 1v1, and 2v2 are best of seven. The champion selector
remains a 3v3 tournament-winner pick; it does not award points.

Matches are grouped by venue calendar day, Tuesday September 15 through Sunday
September 20, then by round. Times use `America/Chicago`, including daylight
saving time, independently of the viewer's timezone. The dates agree with the
[official Worlds primer](https://www.rocketleague.com/news/2026-rocket-league-world-championship-primer).

The schedule tab contains two side-by-side blocks. The importer uses the
rightmost published `Scheduled Start (CT)` block and accepts times with or
without seconds. A blank start time remains “Time to be confirmed.” Sheet
placeholders remain visible but cannot receive predictions until both teams
resolve. Team identity does not depend on the sheet's old 1–16 numeric range.

The play-in sheet also contains unscheduled template finals AG and AM. They
are not in the broadcast schedule and are not imported as playable matches.
The scheduled GSL field has ten matches. Source snapshots from September 14,
2026 are retained in `internal/matchsource/testdata/worlds_*.csv` for tests.

## Scoring and locking

| Completed-match prediction | Points |
| --- | ---: |
| Correct side picked by at most four humans | 4 |
| Other correct pick | 2 |
| Incorrect pick in a series that went the full distance | 1 |
| Other incorrect pick, or no pick | 0 |

`the-coin` and `chat` are scored but excluded from the human underdog tally.
Both retain their match-lock exemptions in the active event. Their upcoming
and live cards remain editable after normal locks; the API also permits
post-result corrections. Completed cards remain read-only in the UI.
`blast_admin` is excluded from scoring and the leaderboard.

Every stage locks match by match when the site imports the first nonzero game
score from the broadcast sheet. Scheduled start times and other matches never
lock an upcoming match. Later rounds can be picked after opponents resolve.
Champion picks lock when any match locks;
Coin and Chat retain their existing exemption. Before completion, other people's
picks are hidden from ordinary viewers; their own picks and admin views remain
available. Archived events expose saved picks for read-only viewing.

Scores, correct-pick counts, team choices, projections, champion picks, and
prediction lists are scoped to the selected event. URLs preserve the selection
with `?event=<id>`, and frontend caches include both event and viewer identities.
Unknown event IDs fail explicitly. Archive writes are rejected on the server,
even for BLAST Admin, Coin, and Chat; the separate owner account can correct archives. Active-event writes cannot target archived matches.

On My Picks, tap a locked match to open a sidebar listing each team's pickers.
Coin and Chat retain their editable buttons and use a separate View predictions
link. They appear in the lists but remain excluded from underdog counts; admin
is omitted as a non-scoring account. The match-picks endpoint refuses unlocked
matches and scopes every lookup to the selected event.

## Local development and running

Requirements: Go 1.22+, Node compatible with Vite 8, and pnpm. Use pnpm, not npm.

On Windows, from the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File ./run.ps1
```

The launcher builds frontend and backend before starting Worlds at
`http://localhost:8080`. It uses `data/rlcs.db`, overriding a Linux database path
in an existing `.env`. Optional arguments: `-Port 8081 -DatabasePath tmp/test.db`.
Keep the terminal open while using the app.

For hot reload, run `go run ./cmd/server` and `pnpm --dir web dev` in separate
terminals with a valid local `DATABASE_PATH`. Vite proxies API calls to port 8080.

Validation:

```powershell
go test ./...
pnpm --dir web build
pnpm --dir web lint
```

Tests cover migration preservation, consistent backups, event isolation,
archived API write rejection, Coin/Chat exemptions, per-match locking, source
validation, stable match IDs across team resolution, and all match formats.

## Upgrading the deployed Major site

Use the existing production SQLite database and persistent volume. GitHub and
the local development database do not contain the completed Major predictions.
Do not replace the production database with the local file or initialize a
new empty volume if you want the actual historical results.

On the first upgraded startup, the server creates a consistent SQLite backup
beside the existing database: `<database>.pre-worlds-<timestamp>.bak`. Startup
stops if this backup fails. Migration 005 runs in a transaction and preserves
accounts, passwords, all matches, all predictions, and champion-pick timestamps.
It associates existing champion history with the existing tournament. If old
champion history exists alongside multiple tournaments, migration fails rather
than guessing which tournament owns it; inspect and assign that data explicitly.

The server then creates or reuses Worlds and marks previous tournaments inactive.
Restarting does not reset Worlds or create duplicate events. Only the active event
is polled. The Major is never repolled by the Worlds process.

For Docker, keep the existing `/var/lib/rlcs-predictions/data:/data` mount and
`DATABASE_PATH=/data/rlcs.db`. Stop the old application before replacing it, then
rebuild the service with `docker compose up -d --build`. Verify both events,
the Major's saved totals, and a successful Worlds sync before broadcast use.
Backups contain the same account data as the database and should stay private.

## Configuration

`.env` is loaded at startup; process environment variables take precedence.

| Variable | Default | Purpose |
| --- | --- | --- |
| `ACTIVE_EVENT` | `worlds-2026` | Active event preset |
| `WORLDS_SPREADSHEET_ID` | Linked Worlds sheet ID | Worlds data source |
| `SHEET_POLL_INTERVAL` | `2m` | Worlds refresh interval |
| `DATABASE_PATH` | `./data/rlcs.db` | Persistent SQLite database |
| `PORT` | `8080` | HTTP port |
| `LOG_LEVEL` | `info` | Logging level |
| `DEV_MODE` | `false` | Enables manual sync endpoint |

Worlds always uses its five-tab sheet preset. Old `SHEET_*_GID`,
`SHEET_SPREADSHEET_ID`, and `LIQUIPEDIA_PAGE` settings cannot accidentally import
the Major into Worlds. `ACTIVE_EVENT=paris-major-2026` retains the legacy importer
for deliberate operator use; it reactivates Paris, so do not use it to browse
history. Use the frontend Event selector instead.

## API

Read `/api/events` to discover IDs and active/archive status. Event-scoped
endpoints accept `?event=<id>` and default to the configured live event.

- `GET /api/matches`, `/api/teams`, `/api/participants`
- `GET /api/matches/{match_id}/picks` (after match lock)
- `GET /api/participants/{id}`, `/api/simulation`, `/api/sync/status`
- `POST /api/login` with `{participant_id, password}`
- `POST /api/reset-password` with `{participant_id, new_password}`
- `PUT /api/participants/{id}/predictions/{match_id}` with `{pick: "A" | "B"}`
- `DELETE /api/participants/{id}/predictions/{match_id}`
- `PUT /api/participants/{id}/winner` with `{team_name}`
- `GET /api/health`; dev-only `POST /api/sync/now`

Self-registration remains disabled. The existing honor-system authentication
has been retained: passwords are stored in SQLite, and the bearer token is a
participant ID. It is not a hardened public authentication system.

After choosing a profile, **Forgot Password?** expands a new-password field.
The reset accepts any non-blank password up to 1024 UTF-8 bytes and changes only
that account's password; predictions and scores remain intact across events.
Recovery deliberately requires no old password, email, or identity verification:
any visitor can reset an ordinary account's password. The owner account is excluded. It does not create
accounts or sign the visitor in automatically, and existing bearer sessions
remain valid under the unchanged honor-system authentication.

## Owner corrections

The small **Admin Login** link at the bottom right of the landing page opens
the separate `owner_admin` login. After signing in, select a participant on
the leaderboard to add, change, or clear picks, including completed matches
and archived events. Tournament-winner picks can also be changed after locking.
Scores and underdog bonuses reflect the corrected picks automatically.

Migration 006 provisions this account on startup. It is hidden from participant
lists and excluded from scoring; password reset is disabled in the API and
absent from its login page. BLAST Admin remains unchanged. Authentication uses
the existing honor-system participant-ID tokens. Owner edits retain participant,
match, event membership, and pick-value validation, but bypass timing and
unresolved-team restrictions; placeholder picks refer to the displayed A/B slots.
