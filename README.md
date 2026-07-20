# API Gateway

A feature-rich API Gateway built from scratch in Go with middleware-based architecture. Runs entirely with Docker Compose.

## Architecture

```
                                    ┌──────────────────┐
                                    │   Prometheus      │
                                    │   :9090           │
                                    └────────┬─────────┘
                                             │ scrape
┌──────────┐     ┌──────────────────────────────────────────────┐     ┌─────────────────┐
│          │     │              API Gateway :8080                │     │ User Service x2  │
│  Client  │────>│                                              │────>│  :8081, :8083    │
│          │     │  CORS -> Metrics -> Tracing -> IPFilter ->   │     └─────────────────┘
└──────────┘     │  StructLog -> Log -> RateLimit -> Auth ->    │
                 │  Retry -> CircuitBreaker -> Cache ->         │     ┌─────────────────┐
                 │  Validation -> Transform -> Proxy            │────>│ Product Service  │
                 │                                              │     │  :8082           │
                 └──────────────┬───────────────┬───────────────┘     └─────────────────┘
                                │               │
                     ┌──────────┴───┐   ┌───────┴──────┐
                     │   MongoDB    │   │    Redis     │
                     │   :27017     │   │    :6379     │
                     │  (logs,keys) │   │ (cache,rate) │
                     └──────────────┘   └──────────────┘
```

## Features

| Feature | Description |
|---------|-------------|
| **Reverse Proxy** | Path-based routing to backend services |
| **Rate Limiting** | Redis sliding window, per-IP limits with headers |
| **JWT Auth** | Bearer token + API key authentication |
| **Circuit Breaker** | Closed/Open/Half-Open state machine |
| **Caching** | Redis response cache with TTL and HIT/MISS headers |
| **Load Balancing** | Round-robin across a route's `targets` list |
| **Request Validation** | JSON body schema validation |
| **Transforms** | Add/remove request/response headers and request body fields |
| **IP Filtering** | Whitelist/blacklist mode |
| **Retry** | Exponential backoff for idempotent requests |
| **Prometheus Metrics** | Request count, latency, cache hit rate, in-flight |
| **Structured Logging** | JSON log output with trace IDs |
| **Distributed Tracing** | OpenTelemetry with OTLP export |
| **Grafana Dashboard** | Pre-configured monitoring dashboard |
| **Admin API** | Manage routes, API keys, view stats |

## Quick Start

```bash
# Clone the repository
git clone https://github.com/MuratZrl/api-gateway.git
cd api-gateway

# Start all services
docker compose up -d --build

# Verify
curl http://localhost:8080/health
```

## API Endpoints

### Gateway

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |
| GET | `/api/users` | List users (proxied) |
| POST | `/api/users` | Create user (proxied) |
| GET | `/api/products` | List products (proxied) |
| POST | `/api/products` | Create product (proxied) |

### Admin

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/routes` | List all routes |
| POST | `/admin/routes` | Add a new route |
| DELETE | `/admin/routes?id=` | Delete a route |
| GET | `/admin/stats` | Request statistics |
| POST | `/admin/keys` | Create API key |
| POST | `/admin/token` | Generate JWT token |

## Usage Examples

```bash
# List users through gateway
curl http://localhost:8080/api/users

# Create a user
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name": "Ali Yilmaz", "email": "ali@example.com"}'

# Create an API key
curl -X POST http://localhost:8080/admin/keys \
  -H "Content-Type: application/json" \
  -d '{"name": "my-service"}'

# Generate JWT token
curl -X POST http://localhost:8080/admin/token \
  -H "Content-Type: application/json" \
  -d '{"user_id": "1", "role": "admin"}'

# Access with JWT
curl http://localhost:8080/api/users \
  -H "Authorization: Bearer <token>"

# Check request stats
curl http://localhost:8080/admin/stats

# View Prometheus metrics
curl http://localhost:8080/metrics
```

## Response Headers

Every API response includes:
- `X-RateLimit-Limit` — Max requests per minute
- `X-RateLimit-Remaining` — Remaining requests
- `X-RateLimit-Reset` — Window reset timestamp
- `X-Cache` — `HIT` or `MISS` (GET requests)
- `X-Powered-By` — API Gateway (`/api/*` routes)
- `X-Trace-ID` — Distributed trace ID (when tracing enabled)

## Monitoring

### Grafana Dashboard
```
URL: http://localhost:3000
User: admin
Password: admin
```

Pre-configured dashboard includes:
- Requests per second
- Request duration (p50, p95)
- Requests in flight
- Cache hit rate
- Rate limit rejections
- HTTP status code distribution
- Response size distribution

### Prometheus
```
URL: http://localhost:9090
```

Available metrics:
- `gateway_http_requests_total` — Total requests by method, path, status
- `gateway_http_request_duration_seconds` — Request latency histogram
- `gateway_http_requests_in_flight` — Current active requests
- `gateway_http_response_size_bytes` — Response size histogram
- `gateway_cache_hits_total` / `gateway_cache_misses_total` — Cache performance
- `gateway_rate_limit_rejections_total` — Rate limit rejections
- `gateway_circuit_breaker_state` — Circuit breaker state per target

## Configuration

All settings are in `configs/gateway.yaml`:

```yaml
server:
  port: 8080

rate_limit:
  requests_per_minute: 60

cache:
  enabled: true
  ttl_seconds: 30

ip_filter:
  mode: "disabled"  # "whitelist", "blacklist", or "disabled"

circuit_breaker:
  max_failures: 5
  timeout: 30

retry:
  max_retries: 3
  multiplier: 2.0

tracing:
  enabled: false
  endpoint: "jaeger:4318"

routes:
  - path: "/api/users"
    target: "http://user-service:8081"
    methods: ["GET", "POST", "PUT", "DELETE"]
    protected: false
```

## Project Structure

```
api-gateway/
├── cmd/gateway/main.go              # Entry point
├── internal/
│   ├── config/                      # YAML config loading
│   ├── gateway/                     # Reverse proxy & load balancer
│   ├── middleware/                   # All middleware
│   │   ├── auth.go                  # JWT + API key
│   │   ├── cache.go                 # Redis caching
│   │   ├── circuitbreaker.go        # Circuit breaker
│   │   ├── ipfilter.go              # IP whitelist/blacklist
│   │   ├── logging.go               # MongoDB request logging
│   │   ├── metrics.go               # Prometheus metrics
│   │   ├── ratelimit.go             # Redis rate limiting
│   │   ├── retry.go                 # Exponential backoff retry
│   │   ├── structlog.go             # JSON structured logging
│   │   ├── tracing.go               # OpenTelemetry tracing
│   │   ├── transform.go             # Request/response transforms
│   │   └── validation.go            # JSON body validation
│   ├── models/                      # MongoDB models
│   ├── repository/                  # MongoDB CRUD
│   └── admin/                       # Admin API handlers
├── services/                        # Example microservices
├── monitoring/                      # Prometheus & Grafana configs
├── docs/openapi.yaml                # OpenAPI specification
├── .github/workflows/               # CI/CD pipelines
├── docker-compose.yml               # Full stack
└── Makefile                         # Common tasks
```

## Development

```bash
# Run unit tests
make test-unit

# Run integration tests (requires docker compose up)
make test-integration

# Generate coverage report
make coverage

# Run linter
make lint

# Build binary
make build
```

## Tech Stack

- **Go** — Gateway and microservices
- **MongoDB** — Request logs, routes, API keys
- **Redis** — Rate limiting, caching
- **Prometheus** — Metrics collection
- **Grafana** — Monitoring dashboard
- **OpenTelemetry** — Distributed tracing
- **Docker Compose** — Container orchestration

## License

MIT
