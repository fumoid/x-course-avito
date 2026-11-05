# Мутационное тестирование domain и usecase

```sh
gremlins unleash --coverpkg=./internal/domain/...,./internal/usecase/... --output docs/mutation/after.json .
```

Gremlins смотрит весь модуль, поэтому в логе много `NOT COVERED` по `cmd/`, `http/`, `infra/`. Ниже только мутанты в `internal/domain` и `internal/usecase`.

## До доработки тестов

Файл: [before.json](before.json).

|          |        |
| -------- | ------ |
| Killed   | 44     |
| Lived    | 2      |
| Efficacy | 95.65% |

Выжившие в домене:

1. `CONDITIONALS_BOUNDARY` `internal/domain/trip/trip.go:94` — `lat < -90` заменяется на `lat <= -90`. Тесты проверяли `90` и `-180`, но не нижнюю границу широты `-90`. Мутант принимал `-90` как ошибку.
2. `CONDITIONALS_BOUNDARY` `internal/domain/trip/trip.go:97` — `lon > 180` заменяется на `lon >= 180`. Не было кейса `longitude = 180`.

## Что добавил

- `lat inclusive -90`, `lat too low`, `lon inclusive 180`, `lon too high` в `internal/domain/trip/trip_test.go`.
- `TestCreateIdempotencyFirstSave`

## После доработки

Файл: [after.json](after.json).

|          |                                               |
| -------- | --------------------------------------------- |
| Killed   | 47                                            |
| Lived    | 0                                             |
| Efficacy | 100% (среди runnable мутантов domain+usecase) |

Выживших в domain и usecase не осталось
