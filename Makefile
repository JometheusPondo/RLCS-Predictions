.PHONY: dev frontend backend build clean

# Unix-style Makefile. On Windows, run from Git Bash or use make.ps1.

dev:
	cd web && pnpm dev & cd .. && go run ./cmd/server

frontend:
	cd web && pnpm install --frozen-lockfile && pnpm run build

backend:
	go build -o bin/server ./cmd/server

build: frontend backend

clean:
	rm -rf web/dist bin/
	mkdir -p web/dist && touch web/dist/.gitkeep
