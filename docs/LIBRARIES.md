# Библиотеки и как они используются

Все зависимости зафиксированы в [go.mod](../go.mod). Инструменты разработки подключены директивой `tool` (Go 1.24+) и запускаются через `go tool <name>` — глобально ставить ничего не нужно, кроме `buf`.

## Runtime-зависимости

| Пакет | Версия | Для чего | Где в коде |
|---|---|---|---|
| `google.golang.org/grpc` | v1.84 | gRPC сервер и клиент, `grpc.health.v1`, reflection | `pkg/grpcx` |
| `google.golang.org/protobuf` | v1.36 | protobuf-сообщения, `protojson`, `timestamppb` | везде |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | v2.31 | REST/JSON → gRPC по аннотациям в proto | `services/gateway/internal/server` |
| `github.com/grpc-ecosystem/go-grpc-middleware/v2` | v2.3 | интерсепторы recovery и logging | `pkg/grpcx/server.go` |
| `github.com/jackc/pgx/v5` | v5.11 | драйвер Postgres, пул `pgxpool`, транзакции | `pkg/postgres`, `repository` auth/shipment/tracking |
| `github.com/pressly/goose/v3` | v3.29 | миграции: CLI и встроенные в бинарник | `pkg/postgres/postgres.go` |
| `github.com/redis/go-redis/v9` | v9.23 | Redis: сессии, кеш, rate limit | `pkg/redisx` и репозитории |
| `github.com/nats-io/nats.go` (+ `jetstream`) | v1.54 | NATS и персистентные стримы | `pkg/natsx` |
| `github.com/golang-jwt/jwt/v5` | v5.3 | выпуск и проверка JWT | `services/auth/internal/service/tokens.go` |
| `golang.org/x/crypto` | v0.58 | `bcrypt` для паролей | `services/auth/internal/service/service.go` |
| `github.com/caarlos0/env/v11` | v11.4 | env → структура конфига | `services/*/internal/config` |
| `github.com/joho/godotenv` | v1.5 | загрузка `.env` при локальном запуске | там же |
| `github.com/google/uuid` | v1.6 | ID сущностей и событий | везде |
| `golang.org/x/sync` | v0.24 | `errgroup` для запуска и остановки компонентов | `pkg/shutdown` |
| `go.opentelemetry.io/otel`, `/sdk`, `/exporters/otlp/otlptrace/otlptracegrpc` | v1.47 | трейсинг и экспорт OTLP/gRPC | `pkg/otelx` |
| `go.opentelemetry.io/contrib/.../otelgrpc` | v0.72 | автотрейсинг gRPC | `pkg/grpcx` |
| `go.opentelemetry.io/contrib/.../otelhttp` | v0.72 | автотрейсинг HTTP в gateway | `services/gateway/internal/server` |
| `github.com/rs/cors` | v1.11 | CORS в gateway | там же |
| `log/slog` (stdlib) | — | структурированные JSON-логи | `pkg/logger` |

## Тесты

| Пакет | Для чего |
|---|---|
| `github.com/stretchr/testify` | `assert` / `require` |
| `github.com/testcontainers/testcontainers-go` + модуль `postgres` | интеграционные тесты с настоящими БД в Docker (build tag `integration`) |

## Инструменты (`go tool <name>`)

| Инструмент | Для чего | Запуск |
|---|---|---|
| `protoc-gen-go` | Go-структуры из proto | через `buf generate` |
| `protoc-gen-go-grpc` | gRPC-стабы | через `buf generate` |
| `protoc-gen-grpc-gateway` | REST-прокси | через `buf generate` |
| `protoc-gen-openapiv2` | OpenAPI (Swagger 2.0) из proto | через `buf generate` |
| `sqlc` | типобезопасный Go-код из SQL | `make sqlc` |
| `goose` | CLI миграций | `make migrate-up` |
| `golangci-lint` (v2) | линтер, конфиг в `.golangci.yml` | `make lint` |

`buf` ставится отдельно (`scoop install buf`, `brew install bufbuild/buf/buf`) и вызывает плагины выше через `go tool` — см. `local: [go, tool, ...]` в [proto/buf.gen.yaml](../proto/buf.gen.yaml). Proto-зависимости (`googleapis`, аннотации `openapiv2`) скачиваются из Buf Schema Registry и зафиксированы в `proto/buf.lock`.

---

## Примеры из кода проекта

### Конфиг: caarlos0/env + godotenv

[services/auth/internal/config/config.go](../services/auth/internal/config/config.go)

```go
type Config struct {
    LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
    GRPCAddr string `env:"AUTH_GRPC_ADDR" envDefault:":50051"`
    DatabaseURL string `env:"AUTH_DATABASE_URL,required"`
    JWT  JWT          // вложенные структуры разбираются без префикса
    OTel otelx.Config // общий кусок конфига из pkg/
}

type JWT struct {
    AccessSecret string        `env:"JWT_ACCESS_SECRET,required"`
    AccessTTL    time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"` // "15m", "168h"
}

func Load() (Config, error) {
    _ = godotenv.Load() // в проде .env нет — это нормально
    return env.ParseAs[Config]()
}
```

Списки — через `envSeparator`: ``CORSOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","` ``.

### Proto + аннотации grpc-gateway и OpenAPI

[proto/auth/v1/auth.proto](../proto/auth/v1/auth.proto)

```proto
service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse) {
    option (google.api.http) = { post: "/v1/auth/login" body: "*" };
    // эндпоинт без авторизации в OpenAPI
    option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {security: {}};
  }
  // без google.api.http метод не попадает в REST — только внутренний gRPC
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
}
```

Path-параметры берутся из полей запроса: `get: "/v1/shipments/{id}"` заполнит `GetShipmentRequest.id`. Генерация — `make proto` (`buf lint && buf generate`), правила линтера — `STANDARD` (имена `XxxRequest`/`XxxResponse`, `ENUM_UNSPECIFIED = 0` и т.д.).

### gRPC-сервер с интерсепторами и трейсингом

[pkg/grpcx/server.go](../pkg/grpcx/server.go)

```go
srv := grpc.NewServer(
    grpc.StatsHandler(otelgrpc.NewServerHandler()),
    grpc.ChainUnaryInterceptor(
        recovery.UnaryServerInterceptor(recoveryOpt), // паника → codes.Internal + лог со стеком
        logging.UnaryServerInterceptor(SlogAdapter(log), logging.WithLogOnEvents(logging.FinishCall)),
    ),
)
hs := health.NewServer()
healthpb.RegisterHealthServer(srv, hs)
reflection.Register(srv)
```

Сервис регистрирует свою реализацию и запускается через errgroup:

```go
grpcSrv, grpcHealth := grpcx.NewServer(log)
authv1.RegisterAuthServiceServer(grpcSrv, grpcserver.New(svc, log))
```

Ошибки домена переводятся в gRPC-коды в одном месте — `toStatus` в `internal/grpcserver`:

```go
case errors.Is(err, service.ErrEmailTaken):
    return status.Error(codes.AlreadyExists, err.Error()) // gateway → HTTP 409
```

### gRPC-клиент и передача пользователя

[services/tracking/internal/service/shipments.go](../services/tracking/internal/service/shipments.go)

```go
conn, err := grpcx.NewClient(cfg.ShipmentGRPCAddr) // insecure + otelgrpc client handler
client := shipmentv1.NewShipmentServiceClient(conn)

// x-user-id пробрасывается дальше: shipment сам проверит владельца
_, err = client.GetShipment(grpcx.WithUserID(ctx, userID), &shipmentv1.GetShipmentRequest{Id: id})
if status.Code(err) == codes.NotFound { ... }
```

На стороне сервера: `userID, err := grpcx.RequireUserID(ctx)`.

### Gateway: grpc-gateway, middleware, OpenAPI

[services/gateway/internal/server/server.go](../services/gateway/internal/server/server.go)

```go
gw := runtime.NewServeMux(
    runtime.WithIncomingHeaderMatcher(headerMatcher), // клиент не может подделать x-user-id
    runtime.WithMetadata(func(_ context.Context, r *http.Request) metadata.MD {
        if id, ok := middleware.UserID(r.Context()); ok { // положил auth-middleware
            return metadata.Pairs(grpcx.UserIDKey, id)
        }
        return nil
    }),
)
_ = authv1.RegisterAuthServiceHandler(ctx, gw, authConn) // готовое соединение, общее с health-check

api := chain(gw, middleware.RateLimit(rdb, 100, time.Minute, log), middleware.Auth(validator, isPublic))
root.HandleFunc("GET /openapi.json", serveSpec) // gen/openapi встроен через embed
handler := otelhttp.NewHandler(chain(root, cors.New(opts).Handler, middleware.Logging(log)), "gateway")
```

### Postgres: pgxpool + sqlc + goose

[services/shipment/queries/shipments.sql](../services/shipment/queries/shipments.sql), [sqlc.yaml](../services/shipment/sqlc.yaml)

```sql
-- name: GetShipmentForUpdate :one
SELECT * FROM shipments WHERE id = $1 FOR UPDATE;
```

`sqlc.yaml` маппит `uuid` → `github.com/google/uuid.UUID`, `timestamptz` → `time.Time` (nullable → `*time.Time`). Использование ([store.go](../services/shipment/internal/repository/store.go)):

```go
pool, err := postgres.NewPool(ctx, cfg.DatabaseURL) // ParseConfig + Ping
store := repository.NewStore(pool)                  // встраивает *db.Queries

sh, err := store.GetShipment(ctx, id)
if errors.Is(err, pgx.ErrNoRows) { return ErrNotFound }

// транзакция: commit при nil, rollback при ошибке
err = store.InTx(ctx, func(q *db.Queries) error {
    cur, err := q.GetShipmentForUpdate(ctx, id)
    ...
    return events.Enqueue(ctx, q, subject, eventID, msg) // та же транзакция
})
```

Уникальность — через код ошибки Postgres ([users.go](../services/auth/internal/repository/users.go)):

```go
if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return User{}, ErrDuplicate
}
```

Миграции: `-- +goose Up` / `-- +goose Down` в одном файле. CLI — `go tool goose -dir services/auth/migrations postgres "$AUTH_DATABASE_URL" up`; встроенный вариант — `postgres.Migrate(ctx, pool, migrations.FS, log)` (goose Provider + `stdlib.OpenDBFromPool` + session locker).

### Идемпотентная вставка (tracking)

[services/tracking/queries/positions.sql](../services/tracking/queries/positions.sql), [positions.go](../services/tracking/internal/repository/positions.go)

```sql
-- name: InsertPosition :execrows
INSERT INTO positions (id, shipment_id, station, lat, lon, source, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO NOTHING;
```

```go
// :execrows → sqlc возвращает число затронутых строк
n, err := q.InsertPosition(ctx, db.InsertPositionParams(p)) // p.ID = ID события
inserted := n == 1 // 0 — событие уже обработано, повтор ничего не меняет
```

### Redis

```go
rdb, err := redisx.New(ctx, cfg.RedisURL) // redis.ParseURL: redis:// или rediss:// (TLS, Upstash)

// refresh-сессия с TTL и одноразовое чтение (services/auth/internal/repository/sessions.go)
rdb.Set(ctx, "refresh:"+tokenID, userID, 7*24*time.Hour)
userID, err := rdb.GetDel(ctx, "refresh:"+tokenID).Result() // redis.Nil → нет сессии

// fixed-window rate limit (services/gateway/internal/middleware/ratelimit.go)
pipe := rdb.TxPipeline()
incr := pipe.Incr(ctx, key)
pipe.Expire(ctx, key, window)
_, err = pipe.Exec(ctx)
```

### NATS JetStream: публикация и чтение

[pkg/natsx](../pkg/natsx)

```go
nc, err := natsx.Connect(cfg.NATS, "shipment", log) // nats.Connect + jetstream.New, бесконечный reconnect
err = nc.EnsureStream(ctx) // CreateOrUpdateStream CARGO: shipment.>, tracking.>

// публикация: protobuf + Nats-Msg-Id + trace context в заголовках + producer span
err = nc.Publish(ctx, natsx.SubjectPositionUpdated, ev.GetEventId(), ev)

// потребление: durable consumer, consumer span продолжает трейс публикатора
cons, err := nc.Consume(ctx, natsx.ConsumerConfig{
    Durable:        "tracking",
    FilterSubjects: []string{natsx.SubjectShipmentCreated},
}, func(ctx context.Context, m jetstream.Msg) error {
    var ev eventsv1.ShipmentCreated
    if err := proto.Unmarshal(m.Data(), &ev); err != nil {
        return nil // битое сообщение: ack и в лог, не ретраим
    }
    return svc.HandleShipmentCreated(ctx, &ev) // ошибка → NakWithDelay, повтор
}, log)
defer cons.Stop(ctx) // Drain
```

Передача трейса через заголовки ([propagation.go](../pkg/natsx/propagation.go)):

```go
type HeaderCarrier nats.Header // реализует propagation.TextMapCarrier

otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(msg.Header))       // publisher
ctx = otel.GetTextMapPropagator().Extract(ctx, HeaderCarrier(m.Headers())) // consumer
```

### Outbox (shipment)

[services/shipment/internal/events](../services/shipment/internal/events)

```go
// в транзакции бизнес-операции
carrier := propagation.MapCarrier{}
otel.GetTextMapPropagator().Inject(ctx, carrier) // сохраняем traceparent
q.InsertOutbox(ctx, db.InsertOutboxParams{ID: eventID, Subject: subject, Payload: payload, Headers: json(carrier)})

// relay, раз в OUTBOX_POLL_INTERVAL
store.InTx(ctx, func(q *db.Queries) error {
    rows, _ := q.LockUnsentOutbox(ctx, 100) // FOR UPDATE SKIP LOCKED
    for _, row := range rows {
        ctx := otel.GetTextMapPropagator().Extract(ctx, headersOf(row)) // продолжаем трейс запроса
        nc.PublishRaw(ctx, row.Subject, row.ID.String(), row.Payload, nil)
    }
    return q.MarkOutboxSent(ctx, sentIDs)
})
```

### JWT

[services/auth/internal/service/tokens.go](../services/auth/internal/service/tokens.go)

```go
claims := jwt.RegisteredClaims{
    ID: uuid.NewString(), Issuer: "cargoflow-auth", Subject: userID,
    Audience:  jwt.ClaimStrings{"access"},
    ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
}
token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)

_, err = jwt.ParseWithClaims(token, &jwt.RegisteredClaims{},
    func(*jwt.Token) (any, error) { return secret, nil },
    jwt.WithValidMethods([]string{"HS256"}), // защита от alg=none и подмены алгоритма
    jwt.WithIssuer("cargoflow-auth"),
    jwt.WithAudience("access"),              // refresh-токен не пройдёт как access
    jwt.WithExpirationRequired(),
)
```

### OpenTelemetry

[pkg/otelx/otelx.go](../pkg/otelx/otelx.go)

```go
shutdown, err := otelx.Init(ctx, "shipment", cfg.OTel)
defer shutdown(context.Background()) // flush оставшихся спанов

// внутри: OTLP/gRPC экспортёр → BatchSpanProcessor → глобальный TracerProvider
exp, _ := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint("localhost:4317"), otlptracegrpc.WithInsecure())
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
otel.SetTracerProvider(tp)
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
```

Пустой `OTEL_EXPORTER_OTLP_ENDPOINT` выключает экспорт, при этом propagator остаётся. Свой спан: `ctx, span := otel.Tracer("name").Start(ctx, "op"); defer span.End()`.

Логгер ([pkg/logger](../pkg/logger/logger.go)) добавляет `trace_id`/`span_id` к записям, сделанным с контекстом: `log.InfoContext(ctx, "route started", "shipment_id", id)`.

### Graceful shutdown

[pkg/shutdown/shutdown.go](../pkg/shutdown/shutdown.go)

```go
ctx, stop := shutdown.SignalContext() // SIGINT, SIGTERM
defer stop()

g := shutdown.New(ctx, 10*time.Second) // errgroup + таймаут на остановку
g.Go(func(ctx context.Context) error { return grpcx.Serve(ctx, grpcSrv, addr) },
    func(ctx context.Context) error { return grpcx.Stop(ctx, grpcSrv) }) // GracefulStop → Stop по таймауту
g.Go(func(ctx context.Context) error { return health.ListenAndServe(ctx, httpSrv) }, httpSrv.Shutdown)
g.Go(relay.Run, nil) // сам завершается по ctx.Done()
return g.Wait()
```

Ошибка любого компонента отменяет общий контекст, и остальные тоже останавливаются.

### Тесты: testify и testcontainers

```go
// unit (services/auth/internal/service/service_test.go): фейковые репозитории через интерфейсы
svc := New(&fakeUsers{...}, &fakeSessions{...}, newTestTokens())
_, err = svc.Refresh(ctx, pair.RefreshToken)
assert.ErrorIs(t, err, ErrInvalidToken)

// интеграционный (services/auth/internal/repository/users_integration_test.go)
//go:build integration
ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("auth"), tcpostgres.BasicWaitStrategies())
testcontainers.CleanupContainer(t, ctr)
dsn, _ := ctr.ConnectionString(ctx, "sslmode=disable")
```

`make test-integration`. На Docker Desktop для Windows контейнер Ryuk (сборщик мусора testcontainers) не стартует, поэтому Makefile на Windows выставляет `TESTCONTAINERS_RYUK_DISABLED=true`. Контейнеры удаляет `CleanupContainer`.
