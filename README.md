# RLCS Prediction Site

A small web app for tracking RLCS broadcast talent match predictions during
in-person Major events. Replaces the spreadsheet workflow: pick a profile,
predict match winners, watch the leaderboard.

**Spec:** see `rlcs-predictions-prompt.md` for the full design document.

## How it works

The server polls a Liquipedia tournament page every 5 minutes, parses the
group-stage matchlists and the playoff bracket, and stores matches in SQLite.
Broadcast talent open the site, pick (or create) a profile, and tap a side of
each match card to predict the winner. Once a match completes on Liquipedia,
that prediction locks and scores the participant if correct. No login — it's
an honor-system tool for a small known group.

One Go binary serves both the JSON API (`/api/*`) and the embedded React
frontend (everything else). No runtime file dependencies except the SQLite
database.

## Stack

- **Backend:** Go 1.22+, [chi](https://github.com/go-chi/chi),
  [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure-Go, no CGO),
  [goquery](https://github.com/PuerkitoBio/goquery), `log/slog`.
- **Frontend:** React 19, TypeScript 6, Vite 8, Tailwind v4, TanStack Query,
  React Router 7.
- **Package manager:** pnpm 9+. Do not use npm — see § 2.1 of the spec for the
  supply-chain hygiene rules.

## Project layout

```
cmd/server/         main — config, DB, poller, HTTP server wiring
internal/
  api/              router, handlers, middleware, SPA file server
  config/           env + .env loading
  db/               SQLite open/migrate + all queries
  liquipedia/       rate-limited client, HTML parser, background poller
  models/           shared structs
embed.go            //go:embed all:web/dist  (root package `app`)
web/                React frontend (Vite project)
  dist/             build output, embedded into the binary (.gitkeep placeholder in git)
Dockerfile          3-stage build → distroless image
Makefile / make.ps1 build targets (Unix / Windows)
```

## Development

Two processes. The Vite dev server proxies `/api/*` to the Go server, so the
browser only ever talks to `localhost:5173`.

```bash
# Terminal 1 — backend (port 8080)
go run ./cmd/server

# Terminal 2 — frontend dev server (port 5173)
cd web && pnpm install && pnpm dev
```

Open `http://localhost:5173`.

On Windows without Git Bash, use `./make.ps1 dev` — it prints the two commands
to run.

### Dev sync endpoint

Set `DEV_MODE=true` to register `POST /api/sync/now`, which triggers a
Liquipedia sync immediately instead of waiting for the 5-minute tick. Off in
production.

## Production build

`make build` (or `./make.ps1 build` on Windows) runs two steps:

1. `pnpm install --frozen-lockfile && pnpm run build` → `web/dist/`
2. `go build -o bin/server ./cmd/server` → single binary with `web/dist/`
   embedded

The resulting `bin/server` is self-contained. It needs only a writable path
for the SQLite database (`DATABASE_PATH`); everything else has a default.

```bash
make build
DATABASE_PATH=./data/rlcs.db ./bin/server
```

### Docker

Three-stage build (Node → Go → distroless), final image under 30 MB:

```bash
docker build -t rlcs-predictions .
docker run -p 8080:8080 \
  -v "$(pwd)/data:/data" \
  -e LIQUIPEDIA_USER_AGENT="RLCSPredictions/1.0 (https://github.com/you/rlcs-predictions; contact: you@example.com)" \
  rlcs-predictions
```

The `-v` mount is required — `DATABASE_PATH` defaults to `/data/rlcs.db` inside
the container, and distroless has no writable filesystem otherwise.

## Environment variables

All optional except where noted; `.env` in the working directory is loaded
automatically (real environment variables take precedence). See `.env.example`.

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_PATH` | `./data/rlcs.db` | SQLite file; parent dir is created if missing. **Must be writable.** |
| `LIQUIPEDIA_PAGE` | `Rocket_League_Championship_Series/2026/Paris_Major` | Page slug to scrape |
| `LIQUIPEDIA_POLL_INTERVAL` | `5m` | Go duration string |
| `LIQUIPEDIA_USER_AGENT` | placeholder | **Set a real contact before production.** Liquipedia bans on missing/bogus contact info. |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `DEV_MODE` | `false` | `true` registers `POST /api/sync/now` |

## Deploy notes

- **User-Agent is mandatory.** Liquipedia's API enforces it and bans on
  violations. The server logs a warning at startup if the placeholder contact
  is still in place.
- **Rate limiting** is enforced inside the Liquipedia client (2-second minimum
  gap between requests), not by the poller — anything that goes through the
  client respects the gate.
- **SQLite WAL mode** is set on every connection. Back up the `.db`, `.db-wal`,
  and `.db-shm` files together, or checkpoint first.
- **Single tournament at a time.** The schema supports multiple tournaments for
  future expansion, but the UI assumes one active tournament (the one in
  `LIQUIPEDIA_PAGE`).
- The container runs as distroless/static — no shell. To inspect the database
  in production, copy the volume out and open it locally.

## API

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/health` | `{"ok":true}` |
| `GET` | `/api/matches` | all matches with embedded round info |
| `GET` | `/api/participants` | all participants, score computed on read |
| `POST` | `/api/participants` | body `{display_name}` → 201; id is a slug of the name, 409 on collision |
| `GET` | `/api/participants/:id` | participant + their predictions |
| `PUT` | `/api/participants/:id/predictions/:match_id` | body `{pick}` (`"A"`/`"B"`) → 200; 400 if match completed |
| `DELETE` | `/api/participants/:id/predictions/:match_id` | 204; 400 if match completed |
| `GET` | `/api/sync/status` | `last_synced_at` + `last_error` |
| `POST` | `/api/sync/now` | dev-only (`DEV_MODE=true`); triggers a sync |

Errors return `{"error": "...", "code": "snake_case_tag"}` with the appropriate
status.
