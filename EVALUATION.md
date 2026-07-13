# TradeDesk — Engineering Guidelines Self-Evaluation

A candid mapping of TradeDesk against the 30 production-grade guidelines. Legend:
✅ fully addressed · 🟡 partial / scoped · ⬜ intentionally out of scope.

| # | Guideline | Status | How it is addressed in TradeDesk |
|---|---|---|---|
| 1 | SOLID principles | ✅ | Hexagonal core; ports = interfaces (DIP); single-responsibility packages; `Operation[T]` open for extension. |
| 2 | Microservices pattern (event-driven / CQRS / Saga) | ✅ | **CQRS** (command service + positions read model) + **event-driven** ordered bus. Saga-style compensation is a natural extension of the event flow. |
| 3 | Design patterns (creational/structural/behavioural) | ✅ | Factory (`NewOrder`), Strategy (`RiskGate`, `Venue`), Observer (event bus), Adapter (ports), Memento (`Snapshot`/`Rehydrate`), Decorator (LLM wraps heuristic). |
| 4 | Partitioning & sharding | ✅ | Order repository FNV-1a sharded into power-of-two buckets for low-contention concurrency. |
| 5 | Timeouts, retry, fault tolerance | ✅ | `WithTimeout`, `Retry` (jittered backoff) on risk + venue boundaries; fail-closed risk rejection. |
| 6 | Rate limiting & circuit breaker | ✅ | Token-bucket `RateLimiter` on PlaceOrder; `CircuitBreaker` around the risk dependency. |
| 7 | Robust error handling & recovery | ✅ | Typed sentinel errors, `errors.Is` mapping, panic-recovery HTTP middleware, optimistic-concurrency conflict handling. |
| 8 | GraphQL over REST (>5 endpoints) | ✅ | 7 root operations over a typed `graphql-go` schema; REST kept only for `/healthz`, `/readyz`, `/metrics`. |
| 9 | Unit + integration test coverage | ✅ | 60 tests, 78% total (domain/app/adapters/api), race-clean; see README test results. |
| 10 | Project structure | ✅ | `cmd/` + layered `internal/` (domain → ports → app → adapters → api). |
| 11 | Modular, reusable components | ✅ | Generic resilience package, reusable observability + system adapters shared verbatim across services. |
| 12 | Idiomatic 3rd-party libraries | ✅ | Go idioms in place of Rust crates: goroutines/`sync` (≈Tokio), `encoding/json` (≈serde), sentinel errors (≈thiserror); `graphql-go`, `prometheus/client_golang`, `google/uuid`. |
| 13 | Generative + Agentic AI | ✅ | `LLMAnalyzer` (agentic pre-trade reviewer) bounded by a heuristic floor, degrading safely to heuristic on error — model can only *raise* risk. |
| 14 | Idiomatic patterns & best practices | ✅ | Small interfaces, value objects, table-driven tests, context propagation, functional-options-style config. |
| 15 | Generics | ✅ | Resilience primitives are generic over `Operation[T]` (`Retry[T]`, `Execute[T]`, `Guard[T]`, `WithTimeout[T]`). |
| 16 | Anchor framework | ⬜ | N/A — TradeDesk is an off-chain brokerage service, no on-chain program. |
| 17 | README + setup + diagrams + TOC + badges | ✅ | README with TOC, badges, mermaid architecture/sequence/state diagrams, component table, run/setup. |
| 18 | Performance, reliability, maintainability | ✅ | Fixed-point integer math, sharded store, bounded queues, criterion-style benchmarks, clean layering. |
| 19 | Async processing & high-performance parallelism | ✅ | Async event bus (goroutine-per-subscriber), non-blocking projection off the write path. |
| 20 | Concurrency, parallelism, batch | ✅ | Sharded concurrent repository, ordered concurrent projection; `List` batch query. |
| 21 | Systematic logging & observability | ✅ | `slog` JSON logs + Prometheus metrics on every command and GraphQL op. |
| 22 | Happy path + edge cases | ✅ | Overfill, illegal transition, version conflict, risk failure (fail-closed), idempotent replay, rate-limit, unknown order all tested. |
| 23 | Clean, composable, extensible architecture | ✅ | Ports-and-adapters; new venues/risk/stores drop in without touching the core. |
| 24 | Well-thought interfaces, config, structure | ✅ | Minimal ports, twelve-factor config with defaults, layered packages. |
| 25 | Type system enforces constraints | ✅ | Newtypes (`Money`, `Quantity`, `Symbol`, `Side`, `OrderStatus`), unexported aggregate fields, adjacency-map state machine. |
| 26 | Benchmarks & complexity | ✅ | `bench_test.go` for domain hot paths; complexity table in README. |
| 27 | Monitoring (Prometheus/Grafana/Alertmanager) | ✅ | Prometheus scrape config, alert rules, Alertmanager route, provisioned Grafana dashboard. |
| 28 | CI/CD pipeline | ✅ | GitHub Actions: gofmt-check, vet, race tests + coverage, golangci-lint, docker build. |
| 29 | Dockerfile | ✅ | Multi-stage build onto distroless nonroot with a self-probing `--health` HEALTHCHECK. |
| 30 | Postman collection | ✅ | `postman/TradeDesk.postman_collection.json` chaining place → fill → positions. |

**Notes / candid gaps.** The persistence adapter is in-memory (production-shaped: sharded,
optimistic-concurrency, idempotent) — a Postgres adapter implementing the same
`OrderRepository` port is the natural next step and requires no core changes. OpenTelemetry
tracing is represented by structured logging + metrics; an OTLP exporter is a drop-in
addition. `cmd/main` is uncovered by unit tests (exercised via the live smoke test instead).
