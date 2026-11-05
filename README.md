# TripGo — лабораторная работа 2

Сервис создаёт, отдаёт и завершает поездки. Снаружи контракт тот же, что в ЛР1:
HTTP API, PostgreSQL, идемпотентность и Docker. Внутри код разложен по слоям,
покрыт тестами, проходит линтер и отдаёт логи, метрики и трейсы в OpenTelemetry.

## Запуск

Нужны Go 1.24, make, Docker и [tripgoctl](https://github.com/course-go-autumn-2026/course-infra).
goose, oapi-codegen и mockgen уже в `go.mod` как `go tool`, ставить отдельно не надо.

golangci-lint **v2.5.0** ставится командой `make lint` через `go run`.

```sh
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
make migrate
make run
```

`environment start` кладёт в `.env` адрес базы и OTLP-коллектора. Makefile читает
`.env.example`, потом `.env`. Обязательна только `DATABASE_URL`.

Сервис на `:8080`. Стоп — Ctrl+C, окружение — `tripgoctl environment stop`.

```sh
make generate          # OpenAPI + mockgen (go generate ./...)
make test              # go test -race, покрытие domain+usecase
make test-integration  # PostgreSQL, билд-тег integration
make lint              # golangci-lint v2.5.0
```

`make generate` из чистого клона:

1. `go tool oapi-codegen` — код из OpenAPI.
2. `go generate ./...` — моки `mockgen` рядом с `contract.go`.

Версия mockgen: `go.uber.org/mock v0.5.2` (`go tool mockgen`).

## Проверка

```sh
curl -i localhost:8080/health
curl -i localhost:8080/ready
```

Обе ручки: `200` и `{"status":"ok"}`.

```sh
curl -i -X POST localhost:8080/api/v1/trips \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'
```

Ожидается `201` и `Location`. Пока поездка active, тот же водитель получит `409 driver_busy`.
`id` из ответа:

```sh
TRIP_ID='<id>'
curl -i localhost:8080/api/v1/trips/$TRIP_ID
curl -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish
curl -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish
```

Второе завершение — `409 trip_completed`. Ошибки поездок — `application/problem+json` с полем `code`.

### Observability

Ссылки на Jaeger и Grafana печатает `tripgoctl connect`. После запроса:

- в логе есть `trace_id` / `span_id` и строка `request handled` с шаблоном маршрута;
- в Jaeger тот же `trace_id`: корневой HTTP-спан и дочерние SQL;
- в Grafana `http_server_request_duration` и `tripgo_trips_created`.

Если коллектор недоступен, сервис продолжает отвечать.

### Idempotency-Key

Сначала завершите поездку из примера, иначе будет `driver_busy`.

```sh
curl -i -X POST localhost:8080/api/v1/trips \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: 7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'
```

Первый раз — `201`. Повтор как есть — `200` и тот же id. С тем же ключом, но другой ценой — `409 idempotency_conflict`.

Ключ живёт 24 часа (`IDEMPOTENCY_TTL`). Повторы срок не продлевают. Когда протух — можно занять снова.

## Docker

Образ: `gcr.io/distroless/static-debian12:nonroot`, внутри только бинарь, не root.

```sh
docker build -f deploy/Dockerfile -t trip-service .
docker run --rm --network host --env-file .env trip-service
```

## Переменные

Полный список в `.env.example`. Обязательна только `DATABASE_URL`.

| Переменная | Дефолт | Зачем |
|---|---|---|
| DATABASE_URL | — | PostgreSQL |
| HTTP_ADDR | :8080 | listen |
| LOG_LEVEL | info | debug/info/warn/error |
| SHUTDOWN_TIMEOUT | 10s | остановка HTTP + пул |
| HTTP_READ_TIMEOUT | 10s | |
| HTTP_READ_HEADER_TIMEOUT | 5s | |
| HTTP_WRITE_TIMEOUT | 15s | |
| HTTP_IDLE_TIMEOUT | 60s | |
| DATABASE_MAX_CONNS | 10 | |
| DATABASE_MIN_CONNS | 2 | |
| DATABASE_MAX_CONN_LIFETIME | 30m | |
| DATABASE_CONNECT_TIMEOUT | 5s | коннект и ping на старте |
| DATABASE_QUERY_TIMEOUT | 3s | запросы, begin/commit/rollback, /ready |
| IDEMPOTENCY_TTL | 24h | сколько живёт ключ |
| OTEL_SERVICE_NAME | trip-service | имя в логах и трейсах |
| OTEL_EXPORTER_OTLP_ENDPOINT | — | адрес коллектора из tripgoctl |
| OTEL_EXPORTER_OTLP_PROTOCOL | grpc | grpc или http/protobuf |
| OTEL_TRACES_SAMPLER | parentbased_always_on | always_on / always_off / traceidratio / parentbased_* |
| OTEL_RESOURCE_ATTRIBUTES | service.namespace=tripgo,deployment.environment=local | ресурс SDK |
| OTEL_SEMCONV_STABILITY_OPT_IN | http/dup | otelhttp: `http.server.request.duration` в секундах |

Нулевой таймаут или `MIN_CONNS > MAX_CONNS` — сервис не стартует.

## Архитектура

Слои из §4.1 задания. Зависимости направлены внутрь: транспорт знает usecase,
usecase знает домен, домен никого не знает.

```
cmd/trip-service          читает конфиг, container.New, ждёт сигнала
internal/config           env
internal/container        единственное место pgxpool.New и конструкторов
internal/domain/trip      статусы, Trip.Finish, доменные ошибки
internal/usecase/trip/{create,get,finish}
internal/http/trip/{create,get,finish}
internal/http/domainerror единый маппинг ошибка → Problem
internal/http/middleware  Recover → Tracing → Logging → Metrics
internal/repository/trip
internal/infra/postgres   пул и SQL-tracer
internal/infra/postgres/transaction
internal/infra/telemetry  OTel SDK и slog
internal/generated
```

Отдельного `internal/model` нет: между слоями ходит `domain/trip.Trip`. Дублировать
поля в DTO незачем — сущность уже описывает поездку, а OpenAPI-типы появляются
только на границе HTTP.

Интерфейсы зависимостей объявлены **неэкспортируемыми** в `contract.go` пакета-потребителя
(usecase, хендлер, middleware), а не рядом с реализацией. Моки генерирует mockgen
в `mocks_test.go` рядом с кодом. Сборка вручную в `container`, без DI-фреймворка
и без `slog.Default()` в бизнес-коде.

`depguard` запрещает домену и usecase тянуть HTTP/pgx/squirrel, usecase — generated
OpenAPI, HTTP-слою — repository и infra.

Переход `active → completed` делает `Trip.Finish(at)` на реальном пути finish.
Одно и то же `at` пишется в строку поездки, историю статусов и ответ.

## Решения

Изоляция Read Committed. Две активные поездки режет partial unique index
`trips_one_active_per_driver_idx`. TxManager.Do кладёт pgx.Tx в context, вложенный
Do вторую транзакцию не открывает. Поездка и история статусов пишутся в одном Do.

Finish: домен решает, можно ли завершить; репозиторий делает `UPDATE ... WHERE status = 'active'`,
чтобы из двух параллельных завершился один.

JSON: сначала ключи (нет поля / null / extra / другой регистр — 400), потом decode.
Нулевая цена нормальная. Нулевой UUID пользователя или водителя — 400, usecase не зовётся.

HTTP-метрика длительности — только `otelhttp`. Счётчик активных запросов свой, с шаблоном
chi (`/api/v1/trips/{tripId}`), без UUID в лейбле. На otelhttp v0.60 без
`OTEL_SEMCONV_STABILITY_OPT_IN=http/dup` уходит старое имя `http.server.duration` в
миллисекундах; с этим флагом появляется `http.server.request.duration` в секундах.
`config.Load` выставляет `http/dup`, если переменная пустая.

SQL инструментируется на пуле своим `QueryTracer`, а не `otelpgx`: у otelpgx часто нет
`db.collection.name` и `error.type`, а задание их требует. Спан — kind `client`.
Гистограмма `db.client.operation.duration` пишется всегда, даже если трейс не семплирован.
Счётчик активных поездок не открывает отдельный корневой SQL-трейс: спан создаётся
только при валидном родительском `SpanContext`.

Порядок middleware из задания: Recover снаружи Tracing, поэтому в `panic recovered`
нет `trace_id`. Тот же запрос виден в `request handled` со статусом 500 и `trace_id`.
Входящий `traceparent` читает стандартный W3C-пропагатор (`otel.SetTextMapPropagator`).

Имя сервиса в логах и трейсах одно — `OTEL_SERVICE_NAME`.
