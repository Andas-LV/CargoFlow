# 3. Карта проекта

## Корень

```
CargoFlow/
├── proto/          контракты API (.proto) — источник правды для всех типов
├── gen/            код, сгенерированный из proto (руками не трогать)
├── services/       5 сервисов — 5 отдельных программ
├── pkg/            общий код, который используют все сервисы
├── deploy/         docker-compose для локалки, render.yaml для продакшна
├── docs/           документация (ты здесь)
├── .github/        CI: проверки на каждый push
├── go.mod, go.sum  зависимости (≈ package.json + lock)
├── Makefile        команды разработчика (≈ scripts в package.json)
├── .env            локальные настройки (в git не попадает)
├── .env.example    шаблон .env с описанием каждой переменной
├── .golangci.yml   настройки линтера (≈ .eslintrc)
├── .dockerignore   что не копировать в Docker-образ (≈ .gitignore для Docker)
└── .gitignore
```

Один `go.mod` на весь репозиторий (монорепо), но каждый сервис собирается в свой бинарник и деплоится отдельно.

## proto/ — контракты

```
proto/
├── buf.yaml           настройки buf: правила линтера, зависимости (googleapis)
├── buf.gen.yaml       какие плагины запускать и куда класть результат
├── buf.lock           зафиксированные версии proto-зависимостей
├── auth/v1/auth.proto           API авторизации
├── shipment/v1/shipment.proto   API заявок
├── tracking/v1/tracking.proto   API маршрутов
└── events/v1/events.proto       формат событий в NATS (не API, а сообщения между сервисами)
```

`buf` — CLI для работы с proto, как `tsc` + eslint для контрактов.

## gen/ — сгенерированное

```
gen/
├── go/<service>/v1/*.go         типы, gRPC-сервер/клиент, REST-прокси (см. главу 2)
└── openapi/
    ├── cargoflow.swagger.json   OpenAPI для фронта
    └── embed.go                 вшивает json в бинарник gateway → GET /openapi.json
```

Коммитится в git, чтобы проект собирался без запуска генераторов.

## services/ — один сервис изнутри

Все сервисы устроены одинаково. Пример — shipment:

```
services/shipment/
├── cmd/main.go            точка входа: читает конфиг, подключает БД/Redis/NATS,
│                          собирает слои вместе, запускает серверы, ждёт Ctrl+C
├── internal/
│   ├── config/            чтение переменных окружения в структуру Config
│   ├── grpcserver/        «контроллер»: принимает gRPC-запрос, достаёт userId,
│   │                      переводит proto ↔ внутренние типы, ошибки → gRPC-коды
│   ├── service/           бизнес-логика: валидация, правила смены статусов,
│   │                      транзакции. Ничего не знает про gRPC и HTTP
│   ├── repository/        доступ к данным: Postgres (через sqlc) и Redis-кеш
│   │   └── db/            ← сгенерировано sqlc из SQL-запросов
│   └── events/            outbox: запись событий и отправка их в NATS
├── migrations/            SQL-миграции схемы БД (+ embed.go, чтобы вшить их в бинарник)
├── queries/               SQL-запросы, из которых sqlc делает Go-функции
├── sqlc.yaml              настройки sqlc
└── Dockerfile             как собрать Docker-образ сервиса
```

Слои в терминах фронта:

| Слой | Аналогия на фронте | Зависит от |
|---|---|---|
| `cmd/main.go` | `main.tsx` с провайдерами: всё создаётся и связывается | всех |
| `grpcserver/` | route handler / контроллер | service |
| `service/` | бизнес-логика, «use cases» | repository (через интерфейсы) |
| `repository/` | слой API-запросов (`api/*.ts`) | БД, Redis |
| `config/` | `import.meta.env` с валидацией | — |

Зачем делить: бизнес-логику (`service`) можно тестировать без gRPC и без базы, подсунув фейковый репозиторий (см. `service_test.go` в auth). Транспорт или базу можно поменять, не трогая логику.

### Чем сервисы отличаются

| Сервис | Особенности |
|---|---|
| **gateway** | нет базы и gRPC-сервера. `internal/server` собирает HTTP-сервер из grpc-gateway, `internal/middleware` — проверка токена, rate limit, логирование |
| **auth** | `service/tokens.go` — выпуск и проверка JWT; `repository/sessions.go` — refresh-сессии в Redis |
| **shipment** | `internal/events` — outbox; `repository/cache.go` — кеш в Redis |
| **tracking** | `internal/consumer` — слушает события из NATS; `service/shipments.go` — gRPC-клиент к shipment |
| **notification** | только `consumer` + `notifier` (заглушки Telegram/email), из HTTP — лишь `/healthz` |

## pkg/ — общий код

| Пакет | Что делает | Аналогия |
|---|---|---|
| `logger` | JSON-логи, добавляет `trace_id` | настроенный `pino` |
| `otelx` | включает трейсинг (OpenTelemetry) | инициализация Sentry |
| `grpcx` | создаёт gRPC-сервер/клиент со всеми настройками, `x-user-id` | общий axios-инстанс с интерсепторами |
| `natsx` | подключение к NATS, publish/consume событий | обёртка над WebSocket/EventSource |
| `postgres` | пул соединений + запуск миграций | — |
| `redisx` | подключение к Redis | — |
| `health` | `/healthz` и `/readyz` | — |
| `shutdown` | корректная остановка по Ctrl+C / SIGTERM | cleanup в `useEffect` |

## deploy/

```
deploy/
├── docker-compose.yml   локальная инфраструктура: Postgres, Redis, NATS, Jaeger
│                        + профиль apps — все 5 сервисов в контейнерах
├── postgres/init.sql    при первом старте создаёт базы auth, shipment, tracking
└── render.yaml          описание деплоя на Render (Infrastructure as Code)
```

## Файлы, которые ты будешь трогать чаще всего

| Хочу… | Файл |
|---|---|
| посмотреть, какие эндпоинты есть | `proto/*/v1/*.proto` или `http://localhost:8080/openapi.json` |
| поменять настройку | `.env` |
| понять логику фичи | `services/<имя>/internal/service/service.go` |
| посмотреть SQL | `services/<имя>/queries/*.sql`, `migrations/*.sql` |

Дальше: [Путь одного запроса →](04-request-journey.md)
