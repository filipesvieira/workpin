CREATE TABLE jobs (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id), customer_id UUID NOT NULL REFERENCES customers(id), service_location_id UUID NOT NULL REFERENCES service_locations(id),
 title TEXT NOT NULL, description TEXT, scheduled_start_at TIMESTAMPTZ NOT NULL, scheduled_end_at TIMESTAMPTZ NOT NULL, status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SCHEDULED','IN_PROGRESS','COMPLETED','CANCELLED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), CHECK (scheduled_end_at > scheduled_start_at)
);
CREATE INDEX jobs_organization_schedule_idx ON jobs (organization_id, scheduled_start_at);
