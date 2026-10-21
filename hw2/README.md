# Marketplace API (ДЗ №2)

Go-сервис с CRUD для товаров, построенный по контракту: сначала пишется
OpenAPI-спецификация, из неё генерируется код, затем пишется логика.
Сделаны пункты 1–6 и 8 задания.

## Запуск

```bash
docker compose up --build
```

Поднимаются три контейнера:

1. `postgres` — PostgreSQL 16.
2. `migrations` — Flyway применяет миграции из `migrations/` и завершается.
3. `app` — сервер на `http://localhost:8080`. Стартует только после успешных миграций.

Посмотреть таблицу:

```bash
docker compose exec postgres psql -U marketplace -d marketplace -c "SELECT * FROM products;"
```

Остановить и удалить данные: `docker compose down -v`.

## Примеры запросов

```bash
# Создать товар
curl -i -X POST localhost:8080/products -H 'Content-Type: application/json' \
  -d '{"name":"Смартфон X100","price":19990.50,"stock":15,"category":"electronics","status":"ACTIVE"}'

# Список с фильтрами и пагинацией
curl 'localhost:8080/products?status=ACTIVE&category=electronics&page=0&size=10'

# Получить, обновить, удалить (мягко)
curl localhost:8080/products/<id>
curl -X PUT localhost:8080/products/<id> -H 'Content-Type: application/json' \
  -d '{"name":"Смартфон X100 Pro","price":24990,"stock":10,"category":"electronics","status":"ACTIVE"}'
curl -X DELETE localhost:8080/products/<id>

# Ошибка валидации: в details.fields перечислены все нарушения
curl -X POST localhost:8080/products -H 'Content-Type: application/json' \
  -d '{"name":"","price":0,"stock":-1,"category":"x","status":"DELETED"}'
```

## Структура

```
openapi/openapi.yaml          контракт API — единственный источник правды
internal/api/                 кодогенерация
  generate.go                   //go:generate — команда генерации
  oapi-codegen.yaml             настройки генератора
  api.gen.go                    сгенерированный код (в .gitignore)
internal/httpapi/             HTTP-слой
  handler.go                    сборка цепочки middleware
  logging.go                    JSON-логи, X-Request-Id (п. 8)
  validation.go                 валидация запросов по спецификации (п. 6)
  server.go                     хендлеры — реализация сгенерированного интерфейса
  errors.go                     единый формат ошибок (п. 5)
internal/product/             доменная модель и SQL-запросы к PostgreSQL
migrations/                   миграции Flyway
cmd/server/                   main: конфиг, подключение к БД, запуск HTTP
```

Путь запроса:

```
логирование → валидация по OpenAPI → сгенерированный роутер → хендлер → репозиторий → PostgreSQL
```

## Как выполнены пункты задания

### 1–2. Спецификация и схемы

Файл [`openapi/openapi.yaml`](openapi/openapi.yaml).

- Пять эндпоинтов CRUD.
- Отдельные схемы `ProductCreate`, `ProductUpdate`, `ProductResponse`.
- `format: decimal` у цены и `format: date-time` у дат.
- Enum статусов, `nullable` у `description`, списки `required`.
- `id`, `created_at` и `updated_at` есть только в ответе. В запросах их нельзя передать: схема запрещает лишние поля через `additionalProperties: false`.

### 3. Кодогенерация

Код генерирует `oapi-codegen` одной командой:

```bash
make generate   # то же самое: go generate ./...
```

Получаются DTO (`ProductCreate`, `ProductResponse`, `ErrorResponse`, …), роутинг и интерфейс `StrictServerInterface`. Хендлеры в `internal/httpapi/server.go` реализуют этот интерфейс.

`api.gen.go` лежит в `.gitignore`. При сборке Docker-образа он генерируется заново, см. `Dockerfile`.

Ручных DTO нет. `product.Product` — это доменная модель для работы с БД, а не DTO API.

Генератор настроен так, чтобы цена превращалась в `decimal.Decimal` (через `x-go-type` в спецификации). `float64` для денег не подходит из-за ошибок округления.

### 4. PostgreSQL и миграции

- Миграции применяет Flyway: [`migrations/V1__create_products.sql`](migrations/V1__create_products.sql).
- Есть индекс `idx_products_status`.
- `created_at` и `updated_at` получают `DEFAULT now()`. `updated_at` дополнительно обновляет триггер при каждом UPDATE.
- `DELETE` делает `UPDATE ... SET status = 'ARCHIVED'`, запись остаётся в таблице. Повторный `DELETE` ничего не меняет и возвращает 200.

### 5. Контрактные ошибки

Все ошибки имеют формат `ErrorResponse` (`error_code`, `message`, `details`).

Каталог кодов описан в спецификации как enum `ErrorCode`. Генератор превратил его в Go-константы, например `api.ErrorCodePRODUCTNOTFOUND`, поэтому опечатка в коде ошибки не скомпилируется.

API товаров использует такие коды:

| Код | HTTP |
|---|---|
| `VALIDATION_ERROR` | 400 |
| `PRODUCT_NOT_FOUND` | 404 |
| `INTERNAL_ERROR` | 500 |

Коды заказов и промокодов уже есть в каталоге и будут использоваться в API заказов.

### 6. Валидация по контракту

Все ограничения (`minLength`, `maxLength`, `minimum`, `maximum`, enum, `required`, типы, формат UUID) записаны только в `openapi.yaml`. В Go-коде их нет.

Middleware `internal/httpapi/validation.go` проверяет каждый запрос по спецификации с помощью `kin-openapi`. Невалидный запрос получает `400 VALIDATION_ERROR` со списком всех нарушений и до хендлера не доходит (это проверяют тесты).

### 8. Логирование

Каждый запрос даёт одну JSON-строку в stdout:

```json
{"timestamp":"2026-10-25T20:33:54.554Z","level":"INFO","msg":"http request",
 "request_id":"ec4048c8-…","method":"POST","endpoint":"/products","status_code":400,
 "duration_ms":0.356,"user_id":null,"request_body":{"name":"","price":-1,…}}
```

- `request_id` — новый UUID на каждый запрос. Он же возвращается в заголовке `X-Request-Id`.
- `request_body` пишется только для POST, PUT и DELETE.
- Поля, в названии которых есть `password`, `token` или `secret`, заменяются на `***`.
- `user_id` всегда `null`, потому что авторизации (п. 9) пока нет.

## Тесты

```bash
make test
```

Тесты в `internal/httpapi/handler_test.go` работают без базы, на хранилище в памяти. Они проверяют:

- что невалидные запросы получают 400 с правильными полями и не доходят до хендлера;
- пагинацию и значения по умолчанию;
- ответы 404 и 500;
- мягкое удаление;
- формат логов и маскирование пароля.
