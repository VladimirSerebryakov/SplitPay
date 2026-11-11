# splitpay

Backend сервиса для учёта совместных трат в группе — аналог [Splitwise](https://www.splitwise.com/).

Пример: вы с друзьями едете в путешествие. За отель платите вы, за ужин — один друг, за такси — другой. splitpay запоминает каждый чек, делит его между участниками (поровну, по процентам, по долям или точными суммами) и показывает, кто кому сколько должен и как рассчитаться минимальным числом переводов.

> **Статус:** учебный pet-проект, в активной разработке. План развития — в [docs/ROADMAP.md](docs/ROADMAP.md).

## Возможности

| Возможность | Статус |
|---|---|
| Регистрация, вход, JWT access + refresh-токены с ротацией | ✅ |
| Rate limiter по пользователю: общий бюджет защищённых запросов в Redis | ✅ |
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
- **Redis**, клиент `go-redis/v9`, Lua-скрипт для счётчика rate limiter
- Миграции — [golang-migrate](https://github.com/golang-migrate/migrate)
- JWT — [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt), пароли — bcrypt
- Валидация — [go-playground/validator](https://github.com/go-playground/validator)
- Тесты — `httptest`, [testcontainers-go](https://golang.testcontainers.org/) для интеграционных

## Быстрый старт

Нужны: Go 1.26+, Docker (с compose), [golang-migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).

```bash
cp -n .env.example .env        # создать только при отсутствии .env; проверьте DATABASE_URL
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d postgres redis
make migrate-up               # применить миграции
make run                      # запустить сервер на :8080
```

Для запуска API нужны обе зависимости: PostgreSQL и Redis. Команда `make db-up` поднимает только PostgreSQL. API проверяет Redis через `PING` при старте; при ошибке запуск завершается. Существующий `.env` сохраняйте; адрес `DATABASE_URL` должен соответствовать параметрам БД.

Остановить окружение: `make db-down` (данные БД сохраняются). Откатить миграции: `make migrate-down`.

### API в Docker, запросы из Windows

Для сборки и запуска API нужны Docker с Compose и `make` в WSL; локальный Go не требуется. Настройте `.env` по примеру `.env.example`, если его ещё нет. Существующий `.env` сохраняйте.

```bash
make docker-up                # собрать образ и запустить API с PostgreSQL и Redis
make docker-logs              # следить за логами API; Ctrl+C завершает просмотр
make docker-down              # остановить только API, оставив PostgreSQL и Redis
```

Контейнер API запускается после успешных healthchecks PostgreSQL и Redis. Go-клиент Redis и rate limiter подключены; Redis должен оставаться доступным для защищённых запросов. Миграции автоматически не применяются: для новой БД сначала выполните `make db-up` и `make migrate-up` (нужен golang-migrate CLI). Если миграции уже применены, повторная подготовка БД не нужна.

Конфигурация разделена на два файла:

- `docker-compose.yml` — сервисы, окружение, healthchecks и volumes. Порты на хост не публикуются; API обращается к PostgreSQL и Redis по внутренней сети Docker.
- `docker-compose.local.yml` — публикация портов для разработки. Команды Compose в Makefile автоматически подключают оба файла.

Эквивалент `make docker-up`:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build api
```

Для запуска зависимостей при работе с Go непосредственно в WSL:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d postgres redis
```

В Windows используйте адрес `http://localhost:8080/api/v1`, например `POST http://localhost:8080/api/v1/auth/sign-in`. Передавайте JSON и заголовок `Content-Type: application/json`. Для регистрации нужны `name`, `email` и `password`.

Порт API на хосте задаётся через `PORT` в `.env`: при `PORT=8081` адрес станет `http://localhost:8081/api/v1`. Внутри контейнера API всегда слушает `8080`. Адрес привязки всех локальных портов задаётся `LOCAL_BIND_ADDR` (по умолчанию `127.0.0.1` — loopback хоста), внешние порты PostgreSQL и Redis — `DB_PORT` и `REDIS_PORT`. Если адрес не открывается, проверьте `docker compose -f docker-compose.yml -f docker-compose.local.yml ps` и логи API; в Windows можно попробовать `http://127.0.0.1:8080/api/v1`.

Если порт `5432` уже занят PostgreSQL в Windows, задайте в `.env` свободный `DB_PORT`, например `15432`. Для локального запуска Go и миграций замените порт в `DATABASE_URL` на тот же. Адрес БД внутри Docker останется `postgres:5432`.

В общей сети Compose API обращается к БД по адресу `postgres:5432`. Строка подключения для контейнера собирается из `DB_USER`, `DB_PASSWORD` и `DB_NAME`; `DATABASE_URL` из `.env` используется для запуска Go и миграций на хосте. При использовании специальных символов в реквизитах учитывайте URL-кодирование строки подключения.

Адрес Redis внутри Docker — `redis:6379`, он передаётся API через `environment`. Для запуска Go из WSL в `.env.example` указан `REDIS_ADDR=localhost:6379`. При изменении внешнего `REDIS_PORT` измените и этот адрес. Значения из `.env` служат для подстановок Compose; переменные приложения, включая `RATE_LIMIT_REQUESTS` и `RATE_LIMIT_WINDOW`, передаются в контейнер явно через `environment`. После изменения настроек в `.env` повторите `make docker-up`, чтобы пересоздать API с новым окружением.

Для будущего запуска на сервере используйте только основной файл:

```bash
docker compose -f docker-compose.yml up -d
```

В этом варианте сервисы доступны внутри сети Docker. Публичный вход через nginx и TLS будет добавлен на этапе деплоя. Redis запускается без пароля; в основном файле его порт на хост не публикуется, а локальное дополнение по умолчанию привязывает его к `127.0.0.1`.

Отдельная сборка образа: `make docker-build` (не требует `.env`). После изменения исходников повторите `make docker-up`. Образ не содержит `.env`; секреты передаются при запуске.

Для текущего локального окружения сохранён PostgreSQL 15 и существующие volumes PostgreSQL и Redis. Версия образа Redis задаётся через `REDIS_IMAGE` (по умолчанию `redis:8-alpine`). Целевая версия PostgreSQL — 16; перенос данных и обновление версии выполняются отдельно. nginx и фоновые процессы пока не запускаются.

### Переменные окружения

| Переменная | Обязательна | Описание |
|---|---|---|
| `DATABASE_URL` | для локального запуска и миграций | Строка подключения к PostgreSQL на хосте |
| `JWT_SECRET` | да | Секрет для подписи access-токенов |
| `PORT` | нет (8080) | Порт HTTP на хосте; внутри Docker API слушает 8080 |
| `LOCAL_BIND_ADDR` | нет (127.0.0.1) | Адрес хоста для публикации локальных портов |
| `REDIS_PORT` | нет (6379) | Локальный порт Redis; внутри Docker всегда 6379 |
| `REDIS_IMAGE` | нет (redis:8-alpine) | Образ Redis |
| `REDIS_ADDR` | да; в контейнере задаётся Compose | В WSL — localhost:6379, в контейнер передаётся redis:6379 |
| `RATE_LIMIT_REQUESTS` | нет (30) | Общий бюджет запросов пользователя; положительное целое число |
| `RATE_LIMIT_WINDOW` | нет (1m) | Длительность окна в формате Go, например `1m` или `30s`; минимум `1ms` |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT` | для docker-compose | Параметры контейнера PostgreSQL |

### Ограничение запросов

По умолчанию пользователь может выполнить **30 запросов за минуту суммарно по всем защищённым маршрутам**. Разные устройства одного пользователя используют общий счётчик. Первый запрос запускает окно; последующие не продлевают его. После истечения окна бюджет восстанавливается. Около границы двух окон возможен всплеск запросов.

Каждое обращение, дошедшее до limiter, учитывается, включая запросы с последующей ошибкой и отклонённые обращения. Публичные auth-маршруты не используют этот счётчик; ограничение по IP в nginx ещё предстоит реализовать. Ошибка Redis при проверке даёт `500 internal`, обработчик не запускается.

При превышении бюджета API возвращает `429` с единым JSON ошибки и заголовком `Retry-After` — оставшимся временем окна в секундах с округлением вверх, минимум `1`:

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 42
```

```json
{
  "error": {
    "code": "rate_limited",
    "message": "Превышен лимит запросов"
  }
}
```

Для ручной проверки при стандартных настройках возьмите действующий access-токен и выполните в WSL:

```bash
API=http://localhost:8080/api/v1
TOKEN='<access_token>'

for i in $(seq 1 31); do
  curl --silent --output /dev/null --write-out '%{http_code}\n' \
    -H "Authorization: Bearer $TOKEN" "$API/me"
done

curl -i -H "Authorization: Bearer $TOKEN" "$API/me"
```

При новом окне первые 30 запросов вернут `200`, 31-й — `429`. Последняя команда покажет заголовок и тело ошибки. Предыдущие запросы этим пользователем могли уже запустить окно и потратить часть бюджета; для проверки дождитесь его окончания. Значение `42` в примере условное — фактическое время зависит от TTL.

## Структура проекта

```
cmd/api/main.go                  точка входа API: конфиг, DI, graceful shutdown
cmd/{relay,*-worker,janitor}/      каталоги будущих фоновых процессов
internal/
  config/                        загрузка конфигурации из env / .env
  domain/                        модели, DTO запросов/ответов, доменные ошибки
  service/                       бизнес-логика; интерфейсы репозиториев объявлены здесь
  repository/postgres/           работа с БД (pgx)
  transport/http/
    router.go                    регистрация маршрутов
    middleware/                  Auth (JWT), Rate Limiter, доступ к группе
    v1/                          HTTP-хендлеры, валидация
  transport/kafka/               будущая работа с Kafka
  events/                        будущие контракты событий
  worker/{balance,activity,notification}/  будущие обработчики событий
  relay/, janitor/               будущие циклы публикации и очистки
  settlement/                    будущие алгоритмы расчёта долгов
  redis/                         реализованное Redis-хранилище счётчиков rate limiter
  token/, s3/, telegram/         операции с токенами и интеграциями (часть ещё предстоит)
migrations/                      SQL-миграции (up/down)
tests/integration/               интеграционные тесты с testcontainers
deploy/nginx/                    каталог будущей конфигурации reverse proxy
swagger.yaml                     спецификация API (OpenAPI)
```

Принятая структура и границы пакетов описаны в [Design.md](docs/Design.md#структура-проекта). Сейчас реализован только прототип API; `.gitkeep` в остальных каталогах сохраняет место для будущего кода.

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
