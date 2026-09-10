# AdFlow Project Instructions

## Project Overview
AdFlow is a Go advertising decision and delivery-validation project. It is a modular monolith with explicit concurrency controls, recoverable event processing, and versioned local performance evidence. Do not describe local validation as production capacity or cluster high-availability verification.

## Architecture Principles

### Domain-Driven Design
- Follow hexagonal architecture (ports and adapters)
- Keep domain logic in `internal/*/domain/`
- Adapters in `internal/*/adapter/`
- Application services in `internal/*/application/`

### Code Organization
```
internal/
  ├── campaign/          # Campaign management context
  ├── decision/          # Core decision engine
  ├── event/             # Event processing
  ├── identity/          # Authentication & authorization
  ├── audit/             # Audit logging
  ├── agentassistant/    # AI rule generation
  └── operations/        # Operational tools
```

## Development Guidelines

### Testing Strategy
- Unit tests for domain logic (targeting 80%+ coverage)
- Integration tests with `//go:build integration` tag
- Load tests in `tests/load/` using k6
- Always run tests before committing: `go test ./cmd/... ./internal/... ./tests/...`; keep ignored `work/` experiments out of the formal package scope

### Performance Considerations
- Use the scenario-specific latency and error thresholds recorded with each experiment; distinguish processing timeouts, interface latency, and statistics-visible latency
- Introduce object pools only after allocation profiles establish a need and ownership is explicit
- Profile with pprof before optimizing: `go test -cpuprofile=cpu.prof -bench=.`
- Cache immutable candidate snapshots; profile caching must follow the existing Cache-Aside invalidation and generation protocol. Do not add uncoordinated mutable user caches

### Concurrency Patterns
- Use `context.Context` for cancellation
- Always set timeouts on database operations
- Use bounded cleanup contexts and owner-checked release. For an uncertain decision commit, query durable state before releasing reservations; only a definite non-commit permits immediate release
- Prefer channels over shared memory where appropriate

### Database
- Use optimistic locking for campaign updates (check `revision`); published rule `version` is a separate historical identifier
- All migrations in `migrations/` directory with checksums
- Require MySQL 8.0; task claims use `FOR UPDATE SKIP LOCKED`
- Index hot paths: decision lookup, outbox claiming, event deduplication

### Error Handling
- Domain errors in `domain/errors.go` with sentinel values
- Never expose internal errors to HTTP clients
- Log errors with context: `slog.Error("operation failed", "error", err, "campaign_id", id)`
- Return domain errors, not implementation details

### Configuration
- All config via environment variables (see `internal/config/config.go`)
- Local development defaults to in-memory adapters
- Persistent Kafka mode requires shared MySQL decision/event storage and Redis reservations. Treat production deployment and high availability as separate validation work
- Never commit secrets or credentials

### HTTP API
- RESTful endpoints with consistent patterns
- Request validation with struct tags
- Return proper HTTP status codes (400/401/403/404/429/500/503/504)
- Include `X-Request-ID` in responses

### Kafka Events
- Use `requestId` as partition key for ordering
- Idempotency via `eventId` at consumer boundary
- Transactional Outbox pattern for durability
- Distinguish relay dead letters, settlement reconciliation and consumer failures. Do not claim every failure automatically enters the same dead-letter path

## When Working on This Project

### Adding New Features
1. Start with domain model in `domain/` package
2. Define ports (interfaces) in `domain/ports.go`
3. Implement adapters separately
4. Write tests before implementation
5. Update OpenAPI spec if adding HTTP endpoints

### Performance Optimization
1. **Always profile first:** `go test -bench=. -benchmem -cpuprofile=cpu.prof`
2. **Use pprof for live profiling:** `.\scripts\profile.ps1 -ProfileType cpu -Duration 30`
3. **Check for allocations:** look for non-zero `B/op` in benchmark results
4. **Monitor database pool:** Check `adflow_db_pool_*` metrics in Prometheus
5. **Object pools require evidence:** Unused preparation pools were removed; only reintroduce them with measured allocation benefits and safe ownership.
6. **Document "before/after" metrics** in `docs/performance-*.md`
7. **Load test after changes:** `k6 run tests/load/delivery-sustained.js`

### Database Performance
- **Connection pool:** Default 30 max, 10 idle. Nine 30/60/100 trials found no stable benefit from raising max-open; see `docs/mysql-pool-comparison-20260908.md`.
- **Monitor wait counts:** Use DBStats window deltas; connection acquisitions include background work and are not unique HTTP requests.
- **Profile queries:** Use `EXPLAIN ANALYZE` for slow queries
- **Keep indexes documented:** Hot paths need proper indexes

### Before Committing
- [ ] Check `gofmt -l cmd internal tests` and format changed Go files when needed
- [ ] Run `go vet ./cmd/... ./internal/... ./tests/...`
- [ ] Run `go test ./cmd/... ./internal/... ./tests/...`
- [ ] Run `go test -race ./cmd/... ./internal/... ./tests/...` for concurrency changes on a supported toolchain; CI also runs race checks
- [ ] Run benchmarks if touching hot paths: `go test -bench=. ./internal/decision/...`
- [ ] Update relevant documentation
- [ ] Check git diff for debug statements or TODOs

## Common Commands

```bash
# Run API locally (in-memory mode)
go run ./cmd/api

# API runs on :18080; profiling is opt-in through ADFLOW_PPROF_ADDR
# Metrics: http://localhost:18080/metrics
# Profiling: http://localhost:6060/debug/pprof/

# Run with real infrastructure
# (Set ADFLOW_CAMPAIGN_REPOSITORY=mysql, etc.)
go run ./cmd/api

# Apply migrations
go run ./cmd/migrate -dir migrations

# Example offered load on an isolated instance; not a capacity claim.
# TARGET_RATE is business iterations/s, not automatically HTTP QPS.
$env:TARGET_RATE = "20"
$env:STEADY_DURATION = "2m"
k6 run tests/load/delivery-sustained.js

# Integration tests (requires MySQL/Redis/Kafka)
go test -tags=integration ./tests/integration

# Benchmarks
go test -bench=. -benchmem ./internal/decision/application

# CPU profiling
go test -bench=BenchmarkDecide -cpuprofile=cpu.prof ./internal/decision/application
go tool pprof -http=:8080 cpu.prof

# Live system profiling
.\scripts\profile.ps1 -ProfileType cpu -Duration 30
.\scripts\profile.ps1 -ProfileType mem
go tool pprof -http=:8080 work/cpu.prof
```

## Evidence and Validation

- Cite the tested version, workload, environment and full result, including failing stages.
- Report interface QPS, complete business iterations and event throughput separately.
- Keep coverage goals separate from measured coverage; do not infer coverage from test counts.
- A passing race check only covers executed paths; it does not prove budget or event consistency.
- Kafka lag is an offset/record count here, not elapsed seconds. Use a separate end-to-end observation for statistics-visible latency.
- Keep personal resume/interview materials in the user's knowledge base or ignored local workspace, not public project documentation.

## Documentation Standards

- Code comments for exported functions
- Complex algorithms need explanation comments
- Performance numbers in `docs/performance-*.md`
- Architecture decisions in `docs/*.md`
- Keep README.md up to date with setup instructions

## Anti-Patterns to Avoid

- ❌ Don't use `panic()` in production code (only in init/config)
- ❌ Don't leak goroutines (always provide cancellation)
- ❌ Don't hold locks across I/O operations
- ❌ Don't use global mutable state
- ❌ Don't ignore context cancellation
- ❌ Don't retry without backoff
- ❌ Don't log sensitive user data
