# 1. Go глазами TypeScript-разработчика

Go — компилируемый язык: `go build` превращает исходники в один исполняемый файл (`bin/auth.exe`), которому для работы не нужны ни Node, ни `node_modules`. Поэтому Docker-образы сервисов весят ~40 МБ.

## Словарь соответствий

| TypeScript / Node | Go | Где в проекте |
|---|---|---|
| `package.json` | `go.mod` — имя модуля, версия Go, зависимости | [go.mod](../../go.mod) |
| `package-lock.json` | `go.sum` — хеши зависимостей | `go.sum` |
| `npm install pkg` | `go get pkg` | — |
| `devDependencies` + `npx tool` | блок `tool (...)` в go.mod + `go tool <name>` | `go tool sqlc` |
| `npm run build` / `npm test` | `go build ./...` / `go test ./...` | Makefile |
| prettier | `gofmt` (встроен, спорить о стиле не о чем) | — |
| eslint | `golangci-lint` | [.golangci.yml](../../.golangci.yml) |
| `index.ts` — точка входа | `func main()` в `cmd/main.go` | `services/*/cmd/main.go` |
| `*.test.ts` | `*_test.go` рядом с кодом | `tokens_test.go` |
| `import x from './x'` (файл) | `import ".../pkg/logger"` (папка целиком) | — |

## Пакет = папка

В Go нет импорта отдельных файлов. Все `.go`-файлы в одной папке — это один **пакет** (как один модуль), они видят друг друга без импортов.

```go
import "github.com/Andas-LV/cargoflow/pkg/logger"  // путь = имя модуля из go.mod + папка

log := logger.New("auth", "debug")                  // обращение через имя пакета
```

**Экспорт — заглавной буквой.** Ключевого слова `export` нет: `New`, `Config`, `UserID` видны снаружи пакета, `parseLevel`, `key` — нет.

**Папка `internal/` — приватная.** Компилятор запрещает импортировать её код из-за пределов родительской папки. `services/auth/internal/...` может использовать только сервис auth, поэтому сервисы не могут случайно залезть в код друг друга. Общий код лежит в `pkg/`.

## Типы: struct и interface

`struct` — как `type` с данными в TS. Методы объявляются отдельно, но «прикрепляются» к типу:

```go
type Cache struct {          // type Cache = { rdb: RedisClient; ttl: number }
    rdb *redis.Client
    ttl time.Duration
}

func (c *Cache) Delete(ctx context.Context, id uuid.UUID) error {  // метод, как в классе
    return c.rdb.Del(ctx, cacheKey(id)).Err()
}
```

`interface` работает как в TS — **структурно**: тип подходит, если у него есть нужные методы. Писать `implements` не нужно.

```go
type SessionStore interface {   // services/auth/internal/service/service.go
    Save(ctx context.Context, tokenID, userID string, ttl time.Duration) error
    Consume(ctx context.Context, tokenID string) (string, error)
}
```

`repository.Sessions` (Redis) подходит под этот интерфейс, а в тестах вместо него подставляется `fakeSessions` (map в памяти). Это как мокать API-модуль в Jest.

## Ошибки — это возвращаемые значения

Исключений (`throw`/`try`) в Go фактически нет. Функция возвращает ошибку вторым значением, и вызывающий код обязан её проверить:

```go
user, err := s.users.GetByEmail(ctx, email)   // как const [user, err] = await ...
if err != nil {
    return TokenPair{}, err                   // пробрасываем наверх
}
```

`if err != nil` встречается на каждом шагу — это нормально. Сравнение ошибок — `errors.Is(err, ErrNotFound)`, примерно как `err instanceof NotFoundError`.

## context.Context

Почти каждая функция первым аргументом принимает `ctx`. Это одновременно:

- **`AbortSignal`** — если клиент закрыл соединение или истёк таймаут, все запросы в БД внутри отменяются;
- **request-scoped данные** — ID пользователя, trace ID (как `AsyncLocalStorage` в Node).

## Параллельность

| JS | Go |
|---|---|
| `async` функция / Promise | горутина: `go doSomething()` — лёгкий поток |
| `Promise.all([...])` + отмена | `errgroup` — см. [pkg/shutdown](../../pkg/shutdown/shutdown.go) |
| `finally` | `defer` — выполнится при выходе из функции |

```go
pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
if err != nil { return err }
defer pool.Close()       // закроется в конце main, что бы ни случилось
```

`await` не нужен: код внутри горутины просто «ждёт» на вызове, как синхронный. Сервер сам обрабатывает каждый запрос в своей горутине.

## Мелочи, которые сбивают с толку

- `:=` — объявить и присвоить (`const`/`let` с выводом типа), `=` — присвоить существующей переменной.
- `*Cache` — указатель («ссылка на объект»), `&x` — взять указатель. На практике: структуры с состоянием передают по указателю.
- **Zero values:** неинициализированная строка — `""`, число — `0`, указатель — `nil`. `undefined` нет.
- **Теги структур** — метаданные для библиотек, как декораторы:
  ```go
  DatabaseURL string `env:"AUTH_DATABASE_URL,required"`  // взять из переменной окружения
  ```
- `//go:embed file.json` — вшить файл в бинарник при сборке (как `import spec from './spec.json'` в бандлере).
- **Кодогенерация — норма.** Файлы с `// Code generated ... DO NOT EDIT.` руками не правят, их пересоздают командой (`make proto`, `make sqlc`).

Дальше: [gRPC за 10 минут →](02-grpc.md)
