# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Test
go test ./...                  # all packages
go test ./cache/...            # specific package
go test -run TestFoo ./...     # specific test
go test -race ./...            # with race detector

# Lint & vet
go vet ./...
golangci-lint run

# Dependencies
go mod tidy
```

## Architecture

`ais-pkg` is a shared Go component library (module: `github.com/aisgo/ais-pkg`) for enterprise services. All packages are designed for zero-invasive integration via **Uber Fx** dependency injection.

### Key Design Patterns

- **Fx modules** — every package exposes `Module` or `FxModule` variables for wiring into apps
- **Strict vs Optional providers** — e.g. `NewClient` (fails startup on error) vs `OptionalNewClient` (graceful degradation)
- **Functional options** — used for query building (`WithPreloads`, `WithScopes`) and message building (`WithKey`, `WithTag`)
- **Interface-first** — `cache.Clienter`, `mq.Producer`, `mq.Consumer` etc. make testing easy with mock implementations

### Package Overview

| Package | Purpose |
|---|---|
| `conf` | Config loading via Viper; supports YAML/JSON + env vars with `${VAR:-default}` syntax |
| `logger` | Zap-based structured logging with file rotation (lumberjack) |
| `database/postgres`, `database/mysql` | GORM connections with Zap logger adapter and connection pool config |
| `cache/redis` | Redis client wrapper with distributed lock support |
| `mq` | Unified producer/consumer abstraction over RocketMQ and Kafka |
| `transport/http` | Fiber v3 HTTP server with health check, Prometheus endpoint, graceful shutdown |
| `middleware` | Fiber middleware: CORS, API key auth, rate limiting, error handling |
| `repository` | Generic CRUD (`RepositoryImpl[T]`), pagination, transactions, multi-tenancy |
| `validator` | go-playground/validator wrapper with struct tag error messages |
| `errors` | `BizError` with error codes compatible with gRPC status codes |
| `response` | Unified `Result` and `PageResult` response structs |
| `ulid` | Distributed sortable ID generation |
| `idempotency` | Redis-backed idempotency checking with best-effort/required/disabled modes |
| `metrics` | Prometheus metrics registration and HTTP exposure |
| `shutdown` | Priority-based graceful shutdown manager with signal handling |

### Error Codes

`errors` package defines codes 1000–1009 that map to both HTTP status and gRPC status codes. The `middleware/error.go` handles this mapping for HTTP responses.

### Configuration

All packages accept YAML-deserializable config structs. See `conf/config.example.yaml` for a complete reference. Environment variables override config with `AIS_<SECTION>_<KEY>` prefix.

### Testing

Integration tests use `testcontainers-go` for real database/Redis instances. Unit tests use `miniredis` for Redis. Test helpers and mocks live alongside production code in `*_test.go` files.
