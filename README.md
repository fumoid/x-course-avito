# TripGo — лабораторная работа 1

Сервис создаёт, отдаёт и завершает поездки. Данные в PostgreSQL.
Сделаны основная часть и оба бонуса: Idempotency-Key и Docker.

## Запуск

Нужны Go 1.24, make, Docker и [tripgoctl](https://github.com/course-go-autumn-2026/course-infra).
goose и oapi-codegen уже в go.mod, ставить отдельно не надо.

```sh
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
make migrate
make run
```

`environment start` кладёт в `.env` адрес базы. Makefile читает `.env.example`, потом `.env`.
Копировать example поверх `.env` не нужно: кроме `DATABASE_URL` у всего есть дефолты.

Сервис на `:8080`. Стоп — Ctrl+C, окружение — `tripgoctl environment stop`.
Переопределить настройку: `make run SHUTDOWN_TIMEOUT=2s` (аргумент make сильнее файлов).

```sh
make generate      # код из OpenAPI
make migrate-down  # откатить одну миграцию
make test          # go test -race ./...  (на ЛР1 тестов почти нет)
```

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
Без заголовка всё как обычно.

Ключ живёт 24 часа (`IDEMPOTENCY_TTL`). Повторы срок не продлевают. Когда протух — можно занять снова.

## Docker

Образ: `gcr.io/distroless/static-debian12:nonroot`, внутри только бинарь, не root.
Размер **12.7 МБ** (`docker images trip-service`).

```sh
docker build -f deploy/Dockerfile -t trip-service .
docker run --rm --network host --env-file .env trip-service
```

`--network host`, чтобы контейнер видел Postgres из tripgoctl на localhost. Так проще на Linux.
Проверка — тот же `/ready`.

## Переменные

Полный список в `.env.example`. Обязательна только `DATABASE_URL`, её пишет tripgoctl.

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

Нулевой таймаут или `MIN_CONNS > MAX_CONNS` — сервис не стартует.

## Решения

Я поставил Read Committed (дефолт Postgres). Две активные поездки режет partial unique index
`trips_one_active_per_driver_idx` на `driver_id WHERE status = 'active'`, а не snapshot,
так что SERIALIZABLE тут ни к чему — только лишние serialization failure.

TxManager.Do кладёт pgx.Tx в context. Репозиторий сам смотрит: есть транзакция — работает через неё,
нет — через пул. Успех — commit, ошибка или паника — rollback. Вложенный Do вторую транзакцию не открывает.
На begin/commit/rollback свой таймаут. Rollback идёт на контексте без отмены, чтобы он доехал,
даже если клиент уже отвалился.

Finish — `UPDATE ... WHERE status = 'active'`. Из двух параллельных завершит один, второй
дочитывает строку и отдаёт 409 или 404.

Поездка и история статусов пишутся в одном Do, так что если история упала — поездки тоже нет.

JSON: сначала смотрю, какие ключи пришли (нет поля / null / extra / другой регистр — 400),
потом decode. Нулевая цена нормальная, если `price` реально передали.

SHUTDOWN_TIMEOUT общий: и HTTP, и закрытие пула.
