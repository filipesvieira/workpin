CREATE TABLE job_assignments (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id), job_id UUID NOT NULL REFERENCES jobs(id), worker_id UUID NOT NULL REFERENCES workers(id),
 hourly_rate_override NUMERIC(12,2) CHECK (hourly_rate_override >= 0), status TEXT NOT NULL DEFAULT 'ASSIGNED' CHECK (status IN ('ASSIGNED','CANCELLED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(job_id, worker_id)
);
CREATE INDEX job_assignments_worker_idx ON job_assignments (organization_id, worker_id);
