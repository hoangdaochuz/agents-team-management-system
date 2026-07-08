## AI Agent Kanban System — developer commands
.PHONY: build test vet lint run tidy compose-up compose-down runner clean web-install web-dev web-build web-typecheck

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

run:
	go run ./cmd/server

tidy:
	go mod tidy

compose-up:
	docker compose up --build

compose-down:
	docker compose down

## Build the per-task runner base image (credential-less sandbox).
runner:
	docker build -t aaks-runner:latest ./runner

clean:
	go clean ./...

## Frontend (Vite + React + TS) — lives in web/.
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-typecheck:
	cd web && npm run typecheck
