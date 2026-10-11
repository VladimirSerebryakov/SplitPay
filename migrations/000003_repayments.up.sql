-- 1. Таблица возвратов долгов (Settle up)
CREATE TABLE repayments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    from_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    to_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Индексы для ускорения поиска по группе и пользователям
CREATE INDEX idx_repayments_group_id ON repayments(group_id);
CREATE INDEX idx_repayments_from_user ON repayments(from_user_id);
CREATE INDEX idx_repayments_to_user ON repayments(to_user_id);
