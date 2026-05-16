# syntax=docker/dockerfile:1
#
# Three-stage build (spec § 8.2):
#   1. node:20-alpine        — build the React frontend
#   2. golang:1.26-alpine    — build the Go binary with the frontend embedded
#   3. distroless/static     — ship only the static binary; final image < 30 MB
#
# Note: spec § 8.2 specified golang:1.22-alpine, but go.mod requires go 1.26.3,
# so the Go stage uses golang:1.26-alpine to match.
#
# modernc.org/sqlite is pure Go, so CGO_ENABLED=0 yields a fully static binary
# that runs on distroless/static with no libc.

# ---- Stage 1: frontend ----
FROM node:20-alpine AS frontend
WORKDIR /app/web

# corepack ships with Node 20; activate pnpm without a global install.
RUN corepack enable && corepack prepare pnpm@latest --activate

# Manifests first — this layer is cached unless dependencies change.
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm run build

# ---- Stage 2: backend ----
FROM golang:1.26-alpine AS backend
WORKDIR /src

# Module graph first — cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Source, then the built frontend from stage 1 into web/dist so //go:embed
# all:web/dist picks up real assets instead of the .gitkeep placeholder.
COPY . .
COPY --from=frontend /app/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# ---- Stage 3: runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=backend /server /server

# DATABASE_PATH must point at a writable mounted volume in production.
ENV DATABASE_PATH=/data/rlcs.db
EXPOSE 8080

ENTRYPOINT ["/server"]
