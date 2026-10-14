# Архитектура CargoFlow

## Сервисы и границы данных

Монорепозиторий с одним `go.mod`, но каждый сервис — отдельный бинарник (`services/<name>/cmd`), отдельный Dockerfile, своя база и независимый деплой.

| Сервис | Владеет данными | Синхронно вызывает | Публикует | Подписан на |
|---|---|---|---|---|
| gateway | счётчики rate limit (Redis `rl:*`) | auth, shipment, tracking | — | — |
| auth | `users` (Postgres `auth`), refresh-сессии (Redis `refresh:*`) | — | — | — |
| shipment | `shipments`, `outbox` (Postgres `shipment`), кеш (Redis `shipment:*`) | — | `shipment.created`, `shipment.status_changed` | — |
| tracking | `positions` (Postgres `tracking`) | shipment (`GetShipment`) | `tracking.position_updated` | `shipment.created` |
| notification | — | — | — | `shipment.>`, `tracking.>` |

Правила:

- Сервис не ходит в чужую БД. Чужие данные — только через gRPC или события. Например, tracking проверяет, что заявка принадлежит пользователю, вызовом `ShipmentService.GetShipment`, а не запросом к Postgres shipment.
- Redis общий, но у каждого сервиса свой префикс ключей.
- Структура сервиса: `cmd/main.go` (сборка зависимостей) → `internal/grpcserver` (proto ↔ домен, коды ошибок) → `internal/service` (логика) → `internal/repository` (sqlc для Postgres, Redis).
- Общий инфраструктурный код — в `pkg/`: logger, otelx, grpcx, natsx, postgres, redisx, health, shutdown.

## Синхронное взаимодействие: gRPC

Контракты — в `proto/<service>/v1/*.proto`, код генерирует `buf generate` в `gen/go`.

```
Browser ──REST──▶ gateway ──gRPC──▶ auth / shipment / tracking
                                       tracking ──gRPC──▶ shipment
```

- **gateway** — единственный публичный сервис. Маршруты REST описаны аннотациями `google.api.http` в proto; grpc-gateway генерирует прокси (`*.pb.gw.go`), а `protoc-gen-openapiv2` — спецификацию `gen/openapi/cargoflow.swagger.json`, которая встраивается в бинарник и отдаётся на `GET /openapi.json`.
- Все gRPC-серверы создаются через `grpcx.NewServer`: OpenTelemetry stats handler, recovery от паник, логирование вызовов через slog, стандартный `grpc.health.v1` и server reflection (удобно для `grpcurl`).
- Клиенты — `grpcx.NewClient`: ленивое подключение, трейсинг, plaintext (трафик идёт только по приватной сети).
- Каждый сервис слушает порт из своего `*_GRPC_ADDR` на всех интерфейсах (`localhost:50051` → `:50051`), поэтому одна переменная служит и адресом для клиентов, и адресом для сервера.

### Аутентификация

1. `POST /v1/auth/login` → auth проверяет bcrypt-хеш и выдаёт пару токенов HS256:
   - access (15 минут, `aud=access`, секрет `JWT_ACCESS_SECRET`);
   - refresh (7 дней, `aud=refresh`, секрет `JWT_REFRESH_SECRET`, `jti` хранится в Redis).
2. На каждый защищённый запрос gateway вызывает `AuthService.ValidateToken` (метод не опубликован в REST). Секрет access-токена знает только auth.
3. gateway кладёт ID пользователя в gRPC-metadata `x-user-id`. Внутренние сервисы доверяют ему, потому что снаружи до них не достучаться. Попытку клиента передать `Grpc-Metadata-X-User-Id` gateway отбрасывает (`headerMatcher` в `services/gateway/internal/server`).
4. `POST /v1/auth/refresh` атомарно забирает сессию из Redis (`GETDEL`), поэтому refresh-токен одноразовый: повторное использование даёт 401.

Защита от перебора email по времени ответа: для несуществующего пользователя login всё равно сравнивает пароль с фиктивным bcrypt-хешем.

## Асинхронное взаимодействие: NATS JetStream

Stream `CARGO`, subjects `shipment.>` и `tracking.>`, file storage, хранение 7 дней, окно дедупликации 2 минуты. Стрим создаётся идемпотентно (`CreateOrUpdateStream`) при старте каждого сервиса, который работает с NATS.

| Subject | Payload (`proto/events/v1`) | Публикует | Потребители (durable) |
|---|---|---|---|
| `shipment.created` | `ShipmentCreated` | shipment, через outbox | `tracking`, `notification` |
| `shipment.status_changed` | `ShipmentStatusChanged` | shipment, через outbox | `notification` |
| `tracking.position_updated` | `PositionUpdated` | tracking, напрямую | `notification` |

- Payload — protobuf (`Content-Type: application/protobuf`). Статусы и типы груза в событиях — строки, чтобы потребители не зависели от enum'ов API.
- Заголовок `Nats-Msg-Id` = ID события: повторная публикация в пределах окна дедупликации отбрасывается JetStream'ом.
- Consumers durable, `AckExplicit`, `MaxDeliver=5`. При ошибке обработчика — `NakWithDelay(2s)`; битые сообщения подтверждаются и логируются, чтобы не крутиться бесконечно.
- Доставка at-least-once, поэтому обработчики идемпотентны. tracking использует ID события как первичный ключ строки (`INSERT ... ON CONFLICT (id) DO NOTHING`), и повтор просто ничего не вставит.

## Transactional outbox (shipment)

Проблема: записать заявку в Postgres и опубликовать событие в NATS атомарно нельзя. Если публиковать после коммита, событие теряется при падении между двумя шагами; если до коммита — уходит событие о несуществующей заявке.

Решение (`services/shipment/internal/events`):

1. `events.Enqueue` в той же транзакции, что и `INSERT INTO shipments` / `UPDATE`, пишет событие в таблицу `outbox`: `id`, `subject`, `payload` (protobuf) и `headers` — W3C trace context текущего запроса в JSONB.
2. `Relay` раз в `OUTBOX_POLL_INTERVAL` открывает транзакцию, берёт до 100 строк `WHERE sent_at IS NULL ... FOR UPDATE SKIP LOCKED`, публикует их в JetStream и проставляет `sent_at`.
3. `SKIP LOCKED` позволяет нескольким репликам shipment разбирать outbox параллельно без двойной отправки.
4. Если процесс упадёт после публикации, но до коммита, строка уйдёт ещё раз. Дубль отсечёт `Nats-Msg-Id`, а на крайний случай потребители идемпотентны.

tracking публикует `tracking.position_updated` напрямую после вставки в Postgres, чтобы пример остался коротким: при недоступном NATS событие потеряется. Для продакшна там нужен такой же outbox, как в shipment.

## Трейсинг

`pkg/otelx.Init` настраивает глобальный `TracerProvider` с экспортом по OTLP/gRPC и propagator W3C TraceContext + Baggage. Если `OTEL_EXPORTER_OTLP_ENDPOINT` пустой, ставится только propagator: трейсы не отправляются, но контекст между сервисами продолжает передаваться.

Как контекст проходит всю цепочку:

| Переход | Механизм |
|---|---|
| браузер → gateway | `otelhttp` (входящий `traceparent`, если есть) |
| gateway → сервисы, tracking → shipment | `otelgrpc` stats handler на клиенте и сервере |
| запрос → outbox → relay | trace context сохраняется в `outbox.headers` и восстанавливается в relay |
| relay / tracking → NATS → consumer | `natsx.Inject` / `natsx.Extract` через заголовки сообщения, спаны `publish <subject>` (producer) и `process <subject>` (consumer) |

В Jaeger создание заявки — один трейс:

```
[gateway]      POST /v1/shipments
  [auth]         AuthService/ValidateToken
  [shipment]     ShipmentService/CreateShipment
    [shipment]     publish shipment.created        ← outbox relay, асинхронно
      [tracking]     process shipment.created
      [notification] process shipment.created
```

Логи (JSON, slog) содержат `trace_id` и `span_id`, если запись сделана с контекстом (`log.InfoContext(ctx, ...)`), поэтому по ID из Jaeger легко найти строки лога.

**Grafana Cloud.** OTLP-gateway Grafana Cloud (`otlp-gateway-*.grafana.net/otlp`) принимает только OTLP/HTTP, а здесь экспортёр gRPC. Варианты:

1. поставить OpenTelemetry Collector / Grafana Alloy (приёмник OTLP/gRPC → экспорт `otlphttp` в Grafana Cloud) и указать его адрес в `OTEL_EXPORTER_OTLP_ENDPOINT`;
2. добавить `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` и выбирать протокол через `OTEL_EXPORTER_OTLP_PROTOCOL`.

## Здоровье и жизненный цикл

- `GET /healthz` — процесс жив, всегда 200.
- `GET /readyz` — проверяет зависимости (Postgres, Redis, NATS; gateway — ещё и `grpc.health.v1` каждого бэкенда), при сбое отдаёт 503 и JSON с ошибкой по каждой зависимости.
- gRPC-сервисы дополнительно отвечают по `grpc.health.v1.Health/Check`.
- Graceful shutdown (`pkg/shutdown`): SIGINT/SIGTERM отменяют контекст, `errgroup` останавливает компоненты, на всё даётся 10 секунд. gRPC — `GracefulStop` (по таймауту `Stop`), HTTP — `Shutdown`, consumers — `Drain`, outbox relay доходит до конца текущей итерации, затем закрываются NATS (drain), пулы БД и сбрасываются спаны.

## Конфигурация

Только переменные окружения: `caarlos0/env` разбирает их в структуру `internal/config.Config` каждого сервиса, а `godotenv` при локальном запуске подхватывает `.env` (уже заданные переменные не перезаписываются). В Docker и на Render файла `.env` нет. Полный список — в [.env.example](../.env.example).

## Миграции

SQL-миграции goose лежат в `services/<svc>/migrations` (auth, shipment, tracking — у каждого своя БД) и встраиваются в бинарник через `embed`. Применяются:

- локально: `make migrate-up` (goose CLI через `go tool`);
- в Docker и на Render: при старте, если `MIGRATE_ON_START=true`. Используется Postgres advisory lock, поэтому одновременный старт нескольких реплик безопасен.

sqlc читает те же файлы миграций как схему, поэтому Go-код запросов всегда соответствует схеме.

## Что намеренно упрощено

- Уведомления уходят только в Telegram и в один чат (`TELEGRAM_CHAT_ID`), а не владельцу заявки (`services/notification/internal/notifier`). Если токен не задан, уведомления пропускаются.
- Нет outbox в tracking (см. выше).
- Кеш shipment — простой read-through с инвалидацией при смене статуса; списки не кешируются.
- Rate limit — fixed window на IP; при недоступном Redis запросы пропускаются.
- Межсервисный трафик без TLS: рассчитан на приватную сеть Docker / Render.
- Нет пагинации по курсору в `ListShipments`, только `pageSize`.
