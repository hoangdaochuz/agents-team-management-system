## AI Agent Kanban System — developer commands
.PHONY: build test vet lint run tidy compose-up compose-down runner clean

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
