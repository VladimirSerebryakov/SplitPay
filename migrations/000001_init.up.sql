-- SplitPay: начальная схема БД (PostgreSQL 13+, golang-migrate).
-- Суммы денег везде в копейках (BIGINT).

-- ===================== Пользователи и аутентификация =====================
CREATE EXTENSION IF NOT EXISTS citext;  -- регистронезависимый email

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT      NOT NULL UNIQUE,          -- Vasya@x.ru = vasya@x.ru
    name          TEXT        NOT NULL,                 -- для ленты: «Вася добавил…»
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE refresh_sessions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id),
    token_hash  TEXT        NOT NULL UNIQUE,            -- поиск по хешу при /auth/refresh
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,                            -- ротация и logout
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_sessions_user ON refresh_sessions(user_id);  -- «выйти со всех устройств»

-- Привязка Telegram для уведомлений (читает Notification worker).
-- Одноразовые токены привязки живут в Redis (TTL 10 мин), в БД их нет.
CREATE TABLE telegram_links (
    user_id     UUID        PRIMARY KEY REFERENCES users(id),  -- один Telegram на пользователя
    chat_id     BIGINT      NOT NULL UNIQUE,                   -- куда слать sendMessage
    linked_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===================== Группы =====================
-- Одна валюта на всё приложение, поэтому поля currency нет.
-- Владелец определяется через group_members.role, а не дублируется в owner_id.
CREATE TABLE groups (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Выхода из группы нет, участники только добавляются.
CREATE TABLE group_members (
    group_id    UUID        NOT NULL REFERENCES groups(id),
    user_id     UUID        NOT NULL REFERENCES users(id),
    role        TEXT        NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX idx_group_members_user ON group_members(user_id);  -- «мои группы»
-- Ровно один владелец на группу:
CREATE UNIQUE INDEX uq_group_owner ON group_members(group_id) WHERE role = 'owner';

-- Одна активная ссылка-приглашение на группу (новая заменяет старую).
CREATE TABLE group_invites (
    group_id    UUID        PRIMARY KEY REFERENCES groups(id),
    token_hash  TEXT        NOT NULL UNIQUE,            -- храним хеш, как и refresh-токены
    created_by  UUID        NOT NULL REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===================== Деньги =====================

CREATE TABLE expenses (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id     UUID        NOT NULL REFERENCES groups(id),
    payer_id     UUID        NOT NULL,                  -- кто заплатил
    created_by   UUID        NOT NULL REFERENCES users(id),  -- кто добавил (только он может удалить)
    amount       BIGINT      NOT NULL CHECK (amount > 0),
    description  TEXT        NOT NULL CHECK (length(description) BETWEEN 1 AND 200),
    split_type   TEXT        NOT NULL CHECK (split_type IN ('equal', 'exact')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,                           -- мягкое удаление
    -- плательщик обязан быть участником этой группы:
    FOREIGN KEY (group_id, payer_id) REFERENCES group_members(group_id, user_id)
);
-- Settlement и лента берут только неудалённые расходы группы:
CREATE INDEX idx_expenses_group_alive ON expenses(group_id) WHERE deleted_at IS NULL;

-- Доли участников. Сумма долей = expenses.amount (проверяется в приложении
-- в той же транзакции; копеечный остаток разыгрывается один раз при создании).
CREATE TABLE expense_splits (
    expense_id  UUID   NOT NULL REFERENCES expenses(id),
    user_id     UUID   NOT NULL REFERENCES users(id),
    amount      BIGINT NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (expense_id, user_id)                   -- один человек — одна доля
);

-- Факт перевода денег (банком или наличными) в любой момент.
-- Записать может отправитель или получатель; удалить — только автор записи (мягко).
CREATE TABLE repayments (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id      UUID        NOT NULL REFERENCES groups(id),
    from_user_id  UUID        NOT NULL,
    to_user_id    UUID        NOT NULL,
    created_by    UUID        NOT NULL REFERENCES users(id),
    amount        BIGINT      NOT NULL CHECK (amount > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,                           -- мягкое удаление
    CHECK (from_user_id <> to_user_id),
    CHECK (created_by IN (from_user_id, to_user_id)),    -- запись делает участник перевода
    FOREIGN KEY (group_id, from_user_id) REFERENCES group_members(group_id, user_id),
    FOREIGN KEY (group_id, to_user_id)   REFERENCES group_members(group_id, user_id)
);
CREATE INDEX idx_repayments_group_alive ON repayments(group_id) WHERE deleted_at IS NULL;

-- Вложения в S3: чек к расходу или скриншот перевода.
CREATE TABLE attachments (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      UUID        NOT NULL REFERENCES users(id),
    s3_key        TEXT        NOT NULL UNIQUE,          -- ключ объекта, не URL
    content_type  TEXT        NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'application/pdf')),
    size_bytes    BIGINT      NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    status        TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'attached')),
    expense_id    UUID        REFERENCES expenses(id),
    repayment_id  UUID        REFERENCES repayments(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expense_id IS NULL OR repayment_id IS NULL), -- к чему-то одному
    CHECK ((status = 'attached') = (expense_id IS NOT NULL OR repayment_id IS NOT NULL))
);
CREATE INDEX idx_attachments_expense   ON attachments(expense_id)   WHERE expense_id IS NOT NULL;
CREATE INDEX idx_attachments_repayment ON attachments(repayment_id) WHERE repayment_id IS NOT NULL;
CREATE INDEX idx_attachments_pending   ON attachments(created_at)   WHERE status = 'pending';  -- Janitor

-- ===================== Проекции воркеров =====================

-- Balance worker. Только для показа; «Рассчитаться» считает честно по expenses + repayments.
CREATE TABLE balances (
    group_id    UUID        NOT NULL REFERENCES groups(id),
    user_id     UUID        NOT NULL REFERENCES users(id),
    net         BIGINT      NOT NULL DEFAULT 0,         -- >0: должны ему, <0: должен он
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);

-- Activity worker. Уникальный event_id = идемпотентность без отдельного inbox.
CREATE TABLE activity (
    id          BIGSERIAL   PRIMARY KEY,
    event_id    UUID        NOT NULL UNIQUE,
    group_id    UUID        NOT NULL REFERENCES groups(id),
    actor_id    UUID        NOT NULL REFERENCES users(id),
    type        TEXT        NOT NULL,                   -- expense_created, expense_deleted, repayment_created, member_joined
    payload     JSONB       NOT NULL,                   -- снимок: имя, сумма, описание
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_activity_feed ON activity(group_id, created_at DESC, id DESC);

-- ===================== Инфраструктура надёжности =====================

-- Idempotency-Key (источник правды; Redis — только кэш).
CREATE TABLE idempotency_keys (
    user_id        UUID        NOT NULL REFERENCES users(id),
    key            TEXT        NOT NULL,
    request_hash   TEXT        NOT NULL,
    response_code  INT,
    response_body  JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);
CREATE INDEX idx_idempotency_created ON idempotency_keys(created_at);  -- Janitor

-- Transactional outbox. id события = event_id в Kafka.
CREATE TABLE outbox (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id  UUID        NOT NULL,                 -- group_id → ключ партиции
    topic         TEXT        NOT NULL,                 -- expenses.events / groups.events / repayments.events
    event_type    TEXT        NOT NULL,                 -- ExpenseCreated, ExpenseDeleted, ...
    payload       JSONB       NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'sent', 'failed')),
    attempts      INT         NOT NULL DEFAULT 0,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ
);
-- Relay выбирает только новые события, индекс остаётся маленьким:
CREATE INDEX idx_outbox_new ON outbox(created_at) WHERE status = 'new';

-- Inbox для Balance worker (и любых будущих консьюмеров).
CREATE TABLE processed_events (
    consumer      TEXT        NOT NULL,                 -- 'balance-worker'
    event_id      UUID        NOT NULL,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
CREATE INDEX idx_processed_events_at ON processed_events(processed_at);  -- Janitor

