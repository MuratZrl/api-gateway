VERSION := $(shell cat VERSION)
BINARY := gateway
DOCKER_IMAGE := api-gateway

.PHONY: build test test-unit test-integration lint coverage docker-build docker-up docker-down release

## Build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/gateway

## Tests
test: test-unit

test-unit:
	go test ./internal/... -count=1 -v

test-integration:
	go test ./tests/ -count=1 -v -timeout=60s

## Coverage
coverage:
	go test ./internal/... -coverprofile=coverage.out -covermode=atomic -count=1
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

## Lint
lint:
	golangci-lint run ./...

## Docker
docker-build:
	docker compose build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v

docker-logs:
	docker compose logs -f

## Release
release:
	@echo "Creating release v$(VERSION)"
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin v$(VERSION)
