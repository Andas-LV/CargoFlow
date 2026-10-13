# 4. Путь одного запроса

Разберём, что происходит, когда фронт создаёт заявку:

```ts
await fetch('http://localhost:8080/v1/shipments', {
  method: 'POST',
  headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ cargoType: 'CARGO_TYPE_WAGON', origin: 'Astana-1', destination: 'Almaty-2' }),
})
```

```mermaid
sequenceDiagram
    participant FE as Браузер
    participant GW as gateway
    participant AU as auth
    participant SH as shipment
    participant DB as Postgres shipment
    participant N as NATS
    participant TR as tracking
    participant NO as notification

    FE->>GW: POST /v1/shipments (JSON + Bearer)
    GW->>AU: gRPC ValidateToken(token)
    AU-->>GW: userId
    GW->>SH: gRPC CreateShipment + metadata x-user-id
    SH->>DB: BEGIN; INSERT shipments; INSERT outbox; COMMIT
    SH-->>GW: CreateShipmentResponse
    GW-->>FE: 200 JSON {shipment: {...}}
    Note over SH,N: через ≤1 секунду, асинхронно
    SH->>N: publish shipment.created
    N->>TR: shipment.created → первая точка маршрута
    N->>NO: shipment.created → «Shipment created»
```

## Шаг 1. gateway: проверки и перевод в gRPC

Файл: [services/gateway/internal/server/server.go](../../services/gateway/internal/server/server.go). Запрос проходит цепочку middleware — как в Express:

1. **otelhttp** — начинает трейс (запись для Jaeger).
2. **CORS** — пропускает только origin из `CORS_ALLOWED_ORIGINS` (твой `http://localhost:5173`).
3. **Logging** — потом напишет в лог метод, путь, статус и время.
4. **RateLimit** — не больше 100 запросов в минуту с одного IP (счётчик в Redis), иначе 429.
5. **Auth** ([middleware/auth.go](../../services/gateway/internal/middleware/auth.go)) — для всех путей, кроме `/v1/auth/*`, берёт токен из `Authorization` и спрашивает у сервиса auth по gRPC: «токен валиден? чей он?». Нет токена или он плохой — 401.
6. **grpc-gateway** — по аннотации `post: "/v1/shipments"` из proto понимает, что это `ShipmentService.CreateShipment`. Превращает JSON в `CreateShipmentRequest`, добавляет `x-user-id` в metadata и делает gRPC-вызов в shipment.

## Шаг 2. shipment: контроллер → логика → база

1. **grpcserver** ([server.go](../../services/shipment/internal/grpcserver/server.go)) — достаёт `userId` из metadata, переводит enum `CARGO_TYPE_WAGON` во внутреннее `"wagon"` и зовёт сервисный слой.
2. **service** ([service.go](../../services/shipment/internal/service/service.go)) — проверяет данные (откуда и куда заполнены и различаются) и в **одной транзакции**:
   - вставляет строку в `shipments`;
   - вставляет событие `shipment.created` в таблицу `outbox`.
3. Транзакция закоммичена — ответ идёт обратно: grpcserver → gateway → JSON → браузер.

Фронт получил ответ. Дальше всё происходит в фоне.

## Шаг 3. Событие уходит в NATS

В shipment крутится фоновый цикл **outbox relay** ([events/outbox.go](../../services/shipment/internal/events/outbox.go)): раз в секунду он берёт неотправленные строки из `outbox`, публикует их в NATS и помечает отправленными. Зачем такая сложность — в [главе 5](05-events-and-infra.md#outbox-зачем-такая-сложность).

## Шаг 4. Подписчики реагируют

- **tracking** ([consumer/consumer.go](../../services/tracking/internal/consumer/consumer.go)) получает `shipment.created` и записывает первую точку маршрута — станцию отправления. Поэтому `GET /v1/shipments/{id}/route` сразу возвращает одну точку.
- **notification** получает то же событие и «отправляет уведомление» (пока просто пишет в лог `"msg":"notification","title":"Shipment created"`).

shipment про них ничего не знает — он просто сообщил «заявка создана». Завтра можно добавить сервис аналитики, подписанный на то же событие, не меняя shipment.

## Как увидеть это своими глазами

1. Создай заявку (curl из README или с фронта).
2. Открой Jaeger: http://localhost:16686 → Service `gateway` → Find Traces → трейс `POST /v1/shipments`.
3. Увидишь «водопад», как вкладка Network в DevTools, только через все сервисы:

```
[gateway]      POST /v1/shipments
  [auth]         AuthService/ValidateToken
  [shipment]     ShipmentService/CreateShipment
    [shipment]     publish shipment.created
      [tracking]     process shipment.created
      [notification] process shipment.created
```

4. В логах любого сервиса есть поле `trace_id` — по нему находишь строки, относящиеся к этому запросу.

## Другие запросы коротко

| Запрос | Путь |
|---|---|
| `POST /v1/auth/login` | gateway (без проверки токена) → auth: найти пользователя в Postgres, сверить bcrypt-хеш пароля, выдать JWT, сохранить refresh-сессию в Redis |
| `GET /v1/shipments/{id}` | gateway → shipment: сначала Redis-кеш, нет — Postgres, затем проверка «заявка твоя?» (чужая → 404) |
| `PATCH /v1/shipments/{id}/status` | gateway → shipment: проверка допустимого перехода (`created → in_transit → delivered`) + событие `shipment.status_changed` через outbox |
| `POST /v1/shipments/{id}/positions` | gateway → tracking → (gRPC) shipment «заявка этого пользователя?» → запись точки → событие `tracking.position_updated` |

Дальше: [События и инфраструктура →](05-events-and-infra.md)
