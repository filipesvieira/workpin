CREATE TABLE attendance_events (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id), attendance_session_id UUID NOT NULL REFERENCES attendance_sessions(id), worker_id UUID NOT NULL REFERENCES workers(id),
 type TEXT NOT NULL CHECK(type IN ('CHECK_IN','CHECK_OUT','ADMIN_EDIT','ADMIN_APPROVAL')), occurred_at TIMESTAMPTZ NOT NULL, latitude DOUBLE PRECISION NOT NULL, longitude DOUBLE PRECISION NOT NULL, accuracy_meters DOUBLE PRECISION NOT NULL,
 expected_latitude DOUBLE PRECISION NOT NULL, expected_longitude DOUBLE PRECISION NOT NULL, distance_from_location_meters DOUBLE PRECISION NOT NULL, verification_status TEXT NOT NULL CHECK(verification_status IN ('VERIFIED','OUTSIDE_GEOFENCE','LOW_ACCURACY','REVIEW_REQUIRED','MANUAL')),
 device_timestamp TIMESTAMPTZ, server_timestamp TIMESTAMPTZ NOT NULL, metadata JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX attendance_events_session_idx ON attendance_events(attendance_session_id,created_at);
