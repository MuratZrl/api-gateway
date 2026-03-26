# Contributing

Thank you for your interest in contributing to this project!

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/<your-username>/api-gateway.git
   cd api-gateway
   ```
3. Create a new branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites

- Go 1.24+
- Docker and Docker Compose
- golangci-lint (optional, for linting)

### Running Locally

```bash
# Start dependencies
docker compose up -d mongodb redis

# Run the gateway
go run ./cmd/gateway

# Or run the full stack
docker compose up -d --build
```

### Running Tests

```bash
# Unit tests
make test-unit

# Integration tests (requires running services)
docker compose up -d --build
make test-integration

# Coverage report
make coverage
```

### Linting

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
make lint
```

## Code Guidelines

- Follow standard Go conventions and idioms
- Use `go fmt` and `go vet` before committing
- Write tests for new features and bug fixes
- Keep middleware focused on a single responsibility
- Use structured logging (`middleware.LogInfo`, `middleware.LogError`)

## Adding a New Middleware

1. Create a new file in `internal/middleware/`
2. Implement the `func(http.Handler) http.Handler` pattern
3. Add it to the middleware chain in `cmd/gateway/main.go`
4. Write unit tests in `internal/middleware/<name>_test.go`
5. Update the configuration if needed (`internal/config/config.go` + `configs/gateway.yaml`)
6. Document it in the README

## Pull Request Process

1. Ensure all tests pass: `make test-unit`
2. Ensure the linter passes: `make lint`
3. Update documentation if you changed behavior
4. Write a clear PR description explaining what and why
5. Keep PRs focused on a single change

## Commit Messages

Use clear, descriptive commit messages:

```
Add WebSocket proxy support

- Implement WebSocket upgrade detection
- Forward WebSocket connections to backend services
- Add connection timeout configuration
```

## Reporting Issues

- Use GitHub Issues
- Include steps to reproduce
- Include expected vs actual behavior
- Include Go version and OS

## Questions?

Open a GitHub issue with the "question" label.
