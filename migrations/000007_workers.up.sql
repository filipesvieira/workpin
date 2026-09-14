CREATE TABLE workers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id), employee_code TEXT, default_hourly_rate NUMERIC(12,2) NOT NULL CHECK (default_hourly_rate >= 0),
    active BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, employee_code)
);
CREATE INDEX workers_organization_id_idx ON workers (organization_id);
