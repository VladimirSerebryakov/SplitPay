CREATE TABLE group_invites (
    group_id UUID PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_group_invites_token ON group_invites(token);
