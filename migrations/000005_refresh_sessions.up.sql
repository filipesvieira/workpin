CREATE TABLE refresh_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    access_token_hash BYTEA NOT NULL UNIQUE,
    refresh_token_hash BYTEA NOT NULL UNIQUE,
    access_expires_at TIMESTAMPTZ NOT NULL,
    refresh_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX refresh_sessions_user_id_idx ON refresh_sessions (user_id);
CREATE INDEX refresh_sessions_access_token_idx ON refresh_sessions (access_token_hash) WHERE revoked_at IS NULL;
CREATE INDEX refresh_sessions_refresh_token_idx ON refresh_sessions (refresh_token_hash) WHERE revoked_at IS NULL;
