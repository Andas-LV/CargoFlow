# 2. gRPC за 10 минут

## Идея

**REST** — ресурсы и HTTP-глаголы: `POST /v1/shipments`, тело в JSON. Контракт (OpenAPI) — опционален, его можно и не писать.

**gRPC** — вызов функции на другом сервере, как будто она локальная: `client.CreateShipment(ctx, req)`. Контракт обязателен — это `.proto`-файл, из него генерируется типизированный код и для сервера, и для клиента.

Если знаком с **tRPC** — это очень похоже: типобезопасный вызов функций между клиентом и сервером. Только контракт описан на отдельном языке (protobuf), поэтому клиент и сервер могут быть на разных языках.

| | REST | gRPC |
|---|---|---|
| Что вызываем | URL + метод | функцию сервиса (`ShipmentService.CreateShipment`) |
| Формат | JSON (текст) | Protobuf (бинарный, компактный и быстрый) |
| Транспорт | HTTP/1.1 или 2 | только HTTP/2 |
| Контракт | по желанию (OpenAPI) | обязателен (`.proto`) |
| Типы на клиенте | пишешь или генерируешь | всегда генерируются |
| Заголовки | headers | metadata |
| Ошибки | HTTP-статусы (404, 401…) | gRPC-коды (`NOT_FOUND`, `UNAUTHENTICATED`…) |
| Браузер | работает напрямую | напрямую — нет (нужен прокси) |

## Почему внутри gRPC, а снаружи REST

- **Внутри** (gateway → auth/shipment/tracking): строгие контракты, быстрый бинарный формат, сгенерированные клиенты — опечатку в имени поля поймает компилятор, а не пользователь.
- **Снаружи** (браузер → gateway): браузеры не умеют «голый» gRPC. Поэтому gateway принимает обычный REST/JSON и переводит его в gRPC. Перевод не пишется руками — его генерирует **grpc-gateway** по аннотациям в `.proto`.

Итог для тебя: на фронте только REST и JSON, никакого gRPC.

## Разбираем .proto

[proto/shipment/v1/shipment.proto](../../proto/shipment/v1/shipment.proto), сокращённо:

```proto
syntax = "proto3";
package shipment.v1;                        // пространство имён + версия API

service ShipmentService {                   // «класс» с удалёнными методами
  rpc CreateShipment(CreateShipmentRequest) returns (CreateShipmentResponse) {
    option (google.api.http) = {            // ← аннотация для grpc-gateway:
      post: "/v1/shipments"                 //   этот метод доступен как POST /v1/shipments
      body: "*"                             //   всё тело JSON → поля запроса
    };
  }
  rpc GetShipment(GetShipmentRequest) returns (GetShipmentResponse) {
    option (google.api.http) = {get: "/v1/shipments/{id}"};  // {id} → поле id
  }
}

enum CargoType {
  CARGO_TYPE_UNSPECIFIED = 0;               // 0 обязан быть «не задано»
  CARGO_TYPE_WAGON = 1;
  CARGO_TYPE_CONTAINER = 2;
}

message CreateShipmentRequest {             // как interface в TS
  CargoType cargo_type = 1;                 // = 1 — НЕ значение, а номер поля
  string origin = 2;
  string destination = 3;
}
```

Что важно понять:

- **Номера полей** (`= 1`, `= 2`) — это идентификаторы в бинарном формате, по ним поле и находится, а не по имени. Переименовать поле можно, менять или переиспользовать номер — нельзя, иначе старые клиенты прочтут мусор.
- **Нет `null`/`undefined`.** Незаполненное поле = zero value (`""`, `0`, первый элемент enum). Поэтому в enum нулевым всегда идёт `..._UNSPECIFIED` — чтобы «не передали» отличалось от «передали WAGON».
- **Имена в JSON — camelCase.** В proto `cargo_type`, в REST-JSON — `cargoType`. Enum в JSON — строкой: `"CARGO_TYPE_WAGON"`.
- **`v1` в пути** — версия API. Ломающие изменения делают в `v2`, старый продолжает работать.
- Методы без `google.api.http` (например `AuthService.ValidateToken`) в REST не попадают — они только для внутренних вызовов.

## Что генерируется

`make proto` → `buf generate` читает `.proto` и создаёт в `gen/`:

| Файл | Что внутри | Аналогия |
|---|---|---|
| `shipment.pb.go` | Go-структуры сообщений и enum'ов | сгенерированные TS-типы |
| `shipment_grpc.pb.go` | интерфейс сервера + готовый клиент | tRPC router type + client |
| `shipment.pb.gw.go` | REST → gRPC прокси для gateway | — |
| `gen/openapi/cargoflow.swagger.json` | OpenAPI-спецификация | из неё ты генерируешь TS-типы |

Сгенерированный интерфейс сервера:

```go
type ShipmentServiceServer interface {
    CreateShipment(context.Context, *CreateShipmentRequest) (*CreateShipmentResponse, error)
    GetShipment(context.Context, *GetShipmentRequest) (*GetShipmentResponse, error)
    ...
}
```

Сервис shipment реализует эти методы в [internal/grpcserver/server.go](../../services/shipment/internal/grpcserver/server.go). Если забыть метод или ошибиться в типах, код не скомпилируется.

А вот так tracking вызывает shipment — выглядит как обычная функция:

```go
client := shipmentv1.NewShipmentServiceClient(conn)
resp, err := client.GetShipment(ctx, &shipmentv1.GetShipmentRequest{Id: shipmentID})
```

## Metadata и коды ошибок

**Metadata** — это заголовки gRPC. Gateway, проверив JWT, кладёт ID пользователя в metadata `x-user-id`, а сервисы читают его оттуда (`grpcx.RequireUserID(ctx)`).

**Коды ошибок** — свой набор вместо HTTP-статусов. Сервис возвращает `status.Error(codes.NotFound, "shipment not found")`, а gateway переводит это в HTTP 404 и JSON:

```json
{"code": 5, "message": "shipment not found", "details": []}
```

| gRPC-код | HTTP |
|---|---|
| `INVALID_ARGUMENT` (3) | 400 |
| `NOT_FOUND` (5) | 404 |
| `ALREADY_EXISTS` (6) | 409 |
| `FAILED_PRECONDITION` (9) | 400 |
| `UNAUTHENTICATED` (16) | 401 |
| `UNAVAILABLE` (14) | 503 |

На фронте смотри и на HTTP-статус, и на `message`.

## Как потыкать gRPC руками

Как Postman для REST — для gRPC есть **grpcurl** (`scoop install grpcurl`) и сам **Postman** (New → gRPC). В сервисах включён reflection, поэтому они сами рассказывают, какие у них методы:

```bash
grpcurl -plaintext localhost:50052 list                        # какие сервисы есть
grpcurl -plaintext localhost:50052 describe shipment.v1.ShipmentService

grpcurl -plaintext -H 'x-user-id: <твой userId>' \
  -d '{"id":"<shipmentId>"}' \
  localhost:50052 shipment.v1.ShipmentService/GetShipment       # вызов в обход gateway
```

Дальше: [Карта проекта →](03-project-map.md)
