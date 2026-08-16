## AI Agent Kanban System — developer commands
.PHONY: build test vet lint tidy compose-up compose-down e2e runner clean web-install web-dev web-build web-typecheck

## Go targets live in backend/. The backend is 11 services under backend/services —
## run the stack with `make compose-up`, not a single binary.
build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

lint:
	cd backend && golangci-lint run

tidy:
	cd backend && go mod tidy

compose-up:
	docker compose -f deploy/docker-compose.yml up --build

compose-down:
	docker compose -f deploy/docker-compose.yml down

## E2E: boot the stack (detached) and run the lifecycle/isolation assertions.
## Requires curl + jq. See deploy/e2e/e2e.sh for the covered assertions.
e2e:
	docker compose -f deploy/docker-compose.yml up --build -d
	./deploy/e2e/e2e.sh

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
