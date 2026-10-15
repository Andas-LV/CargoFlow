# 6. Как что-то поменять

Общий принцип: **сначала контракт, потом код.** Контракт API — `.proto`, контракт базы — миграции и SQL. После их изменения запускаешь генератор, а компилятор Go подсказывает, что ещё нужно поправить.

## Рецепт 1. Добавить поле в заявку (например, вес груза)

**1. База — новая миграция** `services/shipment/migrations/00003_add_weight.sql`:

```sql
-- +goose Up
ALTER TABLE shipments ADD COLUMN weight_kg integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE shipments DROP COLUMN weight_kg;
```

Существующие файлы миграций не редактируй — только добавляй новые. Применить: `make migrate-up`.

**2. SQL-запрос** — в `services/shipment/queries/shipments.sql` добавь колонку в `INSERT` у `CreateShipment`:

```sql
INSERT INTO shipments (id, owner_id, cargo_type, origin_station_id, destination_station_id, status, weight_kg)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
```

`make sqlc` — в структуре `db.Shipment` появится `WeightKg int32`.

**3. Контракт API** — в `proto/shipment/v1/shipment.proto`:

```proto
message Shipment {
  ...
  google.protobuf.Timestamp updated_at = 8;
  int32 weight_kg = 9;                // новый номер, старые не трогаем
}

message CreateShipmentRequest {
  ...
  int32 weight_kg = 4;
}
```

`make proto` — обновятся Go-типы и `gen/openapi/cargoflow.swagger.json`.

**4. Код** — пробрось поле по слоям:

- `internal/grpcserver/server.go`: `req.GetWeightKg()` → во входные данные сервиса; в `toProto` добавь `WeightKg: sh.WeightKg`;
- `internal/service/service.go`: поле в `CreateInput`, валидация (`> 0`), передача в `CreateShipmentParams`.

`go build ./...` покажет, где ещё не хватает правок.

**5. Проверка и фронт** — перезапусти сервис (`make run-all` или `make apps-up`) и перегенерируй TS-типы из `/openapi.json` (команда — в README). В JSON поле будет называться `weightKg`.

## Рецепт 2. Новый эндпоинт

Например, удаление заявки `DELETE /v1/shipments/{id}`:

1. **proto** — новый `rpc` с HTTP-аннотацией и сообщениями `XxxRequest`/`XxxResponse`:
   ```proto
   rpc DeleteShipment(DeleteShipmentRequest) returns (DeleteShipmentResponse) {
     option (google.api.http) = {delete: "/v1/shipments/{id}"};
   }
   message DeleteShipmentRequest { string id = 1; }
   message DeleteShipmentResponse {}
   ```
2. `make proto`. Сборка пока проходит — до реализации метод отвечает ошибкой `UNIMPLEMENTED`.
3. **SQL** — запрос `-- name: DeleteShipment :exec` в `queries/`, затем `make sqlc`.
4. **service** — метод `Delete` с проверкой владельца (по образцу `UpdateStatus`).
5. **grpcserver** — метод `DeleteShipment`: достать userId, распарсить id, вызвать сервис, перевести ошибки в gRPC-коды.
6. gateway трогать не нужно — маршрут появится сам после перезапуска.

## Рецепт 3. Новая настройка в .env

1. Поле в `services/<имя>/internal/config/config.go`:
   ```go
   MaxShipments int `env:"MAX_SHIPMENTS_PER_USER" envDefault:"50"`
   ```
2. Добавь переменную в `.env` и `.env.example` (с комментарием).
3. Для Docker — при необходимости в `deploy/docker-compose.yml`, для прода — в `deploy/render.yaml`.

## Отладка: где смотреть

| Симптом | Куда смотреть |
|---|---|
| Фронт получает CORS-ошибку | `CORS_ALLOWED_ORIGINS` в `.env` — origin фронта должен совпадать точно, включая порт |
| 401 на всё | токен истёк (живёт 15 минут) — сделай `POST /v1/auth/refresh` |
| 503 `auth service unavailable` | auth не запущен: `curl localhost:8081/readyz` |
| Непонятная 500 | логи сервиса: ищи `"level":"ERROR"`, по `trace_id` — трейс в Jaeger |
| Событие «не дошло» | Jaeger: есть ли спан `publish ...`; логи tracking/notification; http://localhost:8222/jsz?consumers=true |
| `bind: Only one usage of each socket address` | порт занят: сервис уже запущен (в Docker или в другом терминале) |
| Сервис не стартует, `config: ...` | не задана обязательная переменная окружения — имя будет в ошибке |

## Чего не делать

- Не редактировать файлы с `// Code generated ... DO NOT EDIT.` (`gen/`, `repository/db/`) — изменения затрутся при следующей генерации.
- Не менять номера полей в `.proto` и не переиспользовать номера удалённых полей.
- Не менять уже применённые миграции — только добавлять новые.
- Не коммитить `.env` (он в `.gitignore`).

## Что почитать дальше

- [Tour of Go](https://go.dev/tour/) — интерактивный тур по языку, ~2 часа.
- [gRPC: Introduction](https://grpc.io/docs/what-is-grpc/introduction/) и [Protobuf: Language Guide (proto3)](https://protobuf.dev/programming-guides/proto3/).
- [ARCHITECTURE.md](../ARCHITECTURE.md) и [LIBRARIES.md](../LIBRARIES.md) этого проекта — то же самое, но подробнее и для бэкендеров.
