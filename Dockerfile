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
#
# Pinned to the 10.x line on purpose: pnpm 11 dropped Node 18/19/20/21 and
# uses the node:sqlite builtin (Node 22.5+ only), so an unpinned pnpm@latest
# crashes on this Node 20 base image with ERR_UNKNOWN_BUILTIN_MODULE. pnpm 10
# runs on Node 20, uses the same lockfileVersion 9.0 as the committed
# pnpm-lock.yaml, and avoids pnpm 11's new minimumReleaseAge/blockExoticSubdeps
# defaults. "latest-10" is pnpm's published dist-tag for the newest 10.x.
RUN corepack enable && corepack prepare pnpm@latest-10 --activate

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

# Memory-constrained build. modernc.org/sqlite/lib is a machine-generated
# pure-Go translation of all of SQLite and is one of the heaviest compiles in
# the Go ecosystem — on a small build host (≈1 GB RAM) the compiler gets
# OOM-killed ("signal: killed").
#
#   GOGC=20      collect at 20% heap growth instead of the default 100% —
#                trades build speed for a much lower peak RSS.
#   GOMAXPROCS=1 caps the compiler's internal parallelism so it isn't holding
#                several function compilations resident at once.
#   -p=1         compile one package at a time — no big package compiles
#                concurrently with another.
#
# Together these drop the peak from ~2.5–3 GB to well under 1 GB. The build is
# slower but fits in a small instance's RAM. If the build host is large,
# these settings only cost some wall-clock time and are otherwise harmless.
RUN GOGC=20 GOMAXPROCS=1 CGO_ENABLED=0 GOOS=linux \
    go build -p=1 -ldflags="-s -w" -o /server ./cmd/server

# ---- Stage 3: runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=backend /server /server

# DATABASE_PATH must point at a writable mounted volume in production.
ENV DATABASE_PATH=/data/rlcs.db
EXPOSE 8080

ENTRYPOINT ["/server"]
