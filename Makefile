# Makefile for Aperture — the trust and execution layer between AI agents and the real world.

BINARY     := bin/aperture-server
CMD        := ./cmd/aperture-server
GO         := go
GOFLAGS    := -trimpath
LDFLAGS    := -ldflags "-s -w"
LINT       := golangci-lint
AIR        := air
DOCKER     := docker
COMPOSE    := docker compose

.PHONY: build test lint run clean docker-build docker-up fmt vet tidy

## build: compile the server binary to bin/aperture-server
build:
	@mkdir -p bin
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY) $(CMD)

## test: run all tests with the race detector
test:
	$(GO) test -race ./...

## lint: run golangci-lint (install if missing)
lint:
	@which $(LINT) > /dev/null 2>&1 || (echo "golangci-lint not found, install from https://golangci-lint.run/usage/install/" && exit 1)
	$(LINT) run ./...

## run: start the server with hot reload via air (install air if missing)
run:
	@which $(AIR) > /dev/null 2>&1 || $(GO) install github.com/air-verse/air@latest
	$(AIR)

## fmt: format all Go source files
fmt:
	$(GO) fmt ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## tidy: tidy go.mod and go.sum
tidy:
	$(GO) mod tidy

## clean: remove build artifacts
clean:
	@rm -rf bin/
	@echo "cleaned"

## docker-build: build the Docker image
docker-build:
	$(DOCKER) build -t aperture:local -f deploy/Dockerfile .

## docker-up: start the full dev stack via Docker Compose
docker-up:
	$(COMPOSE) -f deploy/docker-compose.yml up --build

## help: print this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
