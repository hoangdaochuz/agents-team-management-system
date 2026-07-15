## AI Agent Kanban System — developer commands
.PHONY: build test vet lint run tidy compose-up compose-down runner clean web-install web-dev web-build web-typecheck

## Go targets live in backend/.
build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

lint:
	cd backend && golangci-lint run

run:
	cd backend && go run ./cmd/server

tidy:
	cd backend && go mod tidy

compose-up:
	docker compose -f deploy/docker-compose.yml up --build

compose-down:
	docker compose -f deploy/docker-compose.yml down

## Build the per-task runner base image (credential-less sandbox).
runner:
	docker build -t aaks-runner:latest backend/runner

clean:
	cd backend && go clean ./...

## Frontend (Vite + React + TS) — lives in frontend/.
web-install:
	cd frontend && npm install

web-dev:
	cd frontend && npm run dev

web-build:
	cd frontend && npm run build

web-typecheck:
	cd frontend && npm run typecheck
