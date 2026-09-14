CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    country CHAR(2) NOT NULL,
    timezone TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    default_geofence_radius_meters INTEGER NOT NULL DEFAULT 100 CHECK (default_geofence_radius_meters > 0),
    max_acceptable_gps_accuracy_meters INTEGER NOT NULL DEFAULT 100 CHECK (max_acceptable_gps_accuracy_meters > 0),
    locale TEXT NOT NULL DEFAULT 'en-GB',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
