CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    phone_e164 TEXT NOT NULL UNIQUE CHECK (phone_e164 ~ '^[+][1-9][0-9]{7,14}$'),
    role TEXT NOT NULL CHECK (role IN ('OWNER', 'ADMIN', 'WORKER')),
    active BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_organization_id_idx ON users (organization_id);
