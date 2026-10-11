# splitpay

Backend сервиса для учёта совместных трат в группе — аналог [Splitwise](https://www.splitwise.com/).

Пример: вы с друзьями едете в путешествие. За отель платите вы, за ужин — один друг, за такси — другой. splitpay запоминает каждый чек, делит его между участниками (поровну, по процентам, по долям или точными суммами) и показывает, кто кому сколько должен и как рассчитаться минимальным числом переводов.

> **Статус:** учебный pet-проект, в активной разработке. План развития — в [docs/ROADMAP.md](docs/ROADMAP.md).

## Возможности

| Возможность | Статус |
|---|---|
| Регистрация, вход, JWT access + refresh-токены с ротацией | ✅ |
| Группы: создание, список, детали с участниками | ✅ |
| Приглашение в группу по токену, вступление | ✅ |
| Профиль пользователя (имя) | 🚧 |
| Чеки (расходы): поровну / проценты / доли / точные суммы | 🚧 |
| Возвраты долгов (settle up) | 🚧 |
| Балансы участников и лента операций | 🚧 |
| Упрощение долгов («кто кому сколько перевести») | 🚧 |

## Стек

- **Go 1.26**, стандартный `net/http` (роутинг через `ServeMux` с паттернами `"POST /path/{id}"`)
- **PostgreSQL 15**, драйвер [pgx/v5](https://github.com/jackc/pgx) (pgxpool)
- Миграции — [golang-migrate](https://github.com/golang-migrate/migrate)
- JWT — [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt), пароли — bcrypt
- Валидация — [go-playground/validator](https://github.com/go-playground/validator)
- Тесты — `httptest`, [testcontainers-go](https://golang.testcontainers.org/) для интеграционных

## Быстрый старт

Нужны: Go 1.26+, Docker (с compose), [golang-migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).

```bash
cp .env.example .env          # проверьте, что DATABASE_URL совпадает с DB_USER/DB_PASSWORD
make db-up                    # поднять PostgreSQL в Docker
make migrate-up               # применить миграции
make run                      # запустить сервер на :8080
```

Остановить БД: `make db-down`. Откатить миграции: `make migrate-down`.

### Переменные окружения

| Переменная | Обязательна | Описание |
|---|---|---|
| `DATABASE_URL` | да | Строка подключения к PostgreSQL |
| `JWT_SECRET` | да | Секрет для подписи access-токенов |
| `PORT` | нет (8080) | Порт HTTP-сервера |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT` | для docker-compose | Параметры контейнера PostgreSQL |

## Структура проекта

```
cmd/server/main.go               точка входа: конфиг, DI, роуты, graceful shutdown
internal/
  config/                        загрузка конфигурации из env / .env
  domain/                        модели, DTO запросов/ответов, доменные ошибки
  service/                       бизнес-логика; интерфейсы репозиториев объявлены здесь
  repository/postgres/           работа с БД (pgx)
  transport/http/
    middleware/                  RequireAuth (JWT), RequireGroupAccess (членство в группе)
    v1/                          HTTP-хендлеры, валидация
migrations/                      SQL-миграции (up/down)
tests/integration/               интеграционные тесты с testcontainers
swagger.yaml                     спецификация API (OpenAPI)
```

## API

Базовый путь: `/api/v1`. Защищённые эндпоинты требуют заголовок `Authorization: Bearer <access_token>`.
Полная спецификация — [swagger.yaml](swagger.yaml) (откройте в [Swagger Editor](https://editor.swagger.io/)).

**Все денежные суммы — целые числа в минимальных единицах валюты (копейках):** `150050` = 1500,50 ₽.

| Метод | Путь | Доступ | Статус |
|---|---|---|---|
| POST | `/auth/sign-up` | — | ✅ |
| POST | `/auth/sign-in` | — | ✅ |
| POST | `/auth/refresh` | — | ✅ |
| POST | `/groups` | авторизация | ✅ |
| GET | `/groups` | авторизация | ✅ |
| POST | `/groups/join` | авторизация | ✅ |
| GET | `/groups/{id}` | участник группы | ✅ |
| POST | `/groups/{id}/invites` | участник группы | ✅ |
| POST | `/groups/{id}/expenses` | участник группы | 🚧 |
| DELETE | `/groups/{id}/expenses/{expense_id}` | участник группы | 🚧 |
| POST | `/groups/{id}/repayments` | участник группы | 🚧 |
| GET | `/groups/{id}/balances` | участник группы | 🚧 |
| GET | `/groups/{id}/transactions` | участник группы | 🚧 |

### Пример сценария

```bash
API=http://localhost:8080/api/v1

# регистрация и вход
curl -X POST $API/auth/sign-up -d '{"email":"me@example.com","password":"secret123"}'
curl -X POST $API/auth/sign-in -d '{"email":"me@example.com","password":"secret123"}'
# → {"access_token":"...","refresh_token":"..."}
TOKEN=<access_token>

# создать группу и получить инвайт
curl -X POST $API/groups -H "Authorization: Bearer $TOKEN" -d '{"name":"Поездка на Алтай"}'
curl -X POST $API/groups/<group_id>/invites -H "Authorization: Bearer $TOKEN"
# → {"invite_link":"<invite_token>","expires_at":"..."}

# друг вступает в группу по токену (со своим access-токеном)
curl -X POST $API/groups/join -H "Authorization: Bearer $FRIEND_TOKEN" -d '{"token":"<invite_token>"}'

# обновить пару токенов
curl -X POST $API/auth/refresh -d '{"refresh_token":"<refresh_token>"}'
```

## Тесты

```bash
go test ./internal/...            # unit-тесты хендлеров (без БД)
go test ./tests/integration/...   # интеграционные тесты репозиториев (нужен запущенный Docker)
```
