CREATE TABLE auth_otps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    code_hash BYTEA NOT NULL,
    code_salt BYTEA NOT NULL,
    attempts SMALLINT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX auth_otps_user_created_idx ON auth_otps (user_id, created_at DESC);
