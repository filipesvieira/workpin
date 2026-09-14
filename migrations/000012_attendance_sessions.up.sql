CREATE TABLE attendance_sessions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id), job_id UUID NOT NULL REFERENCES jobs(id), assignment_id UUID NOT NULL REFERENCES job_assignments(id), worker_id UUID NOT NULL REFERENCES workers(id),
 checkin_at TIMESTAMPTZ NOT NULL, checkout_at TIMESTAMPTZ, worked_seconds BIGINT, hourly_rate NUMERIC(12,2) NOT NULL, calculated_amount NUMERIC(12,2), status TEXT NOT NULL DEFAULT 'OPEN' CHECK(status IN ('OPEN','COMPLETED','REVIEW_REQUIRED')), approved_at TIMESTAMPTZ, approved_by UUID REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX attendance_sessions_org_worker_idx ON attendance_sessions(organization_id,worker_id,checkin_at);
