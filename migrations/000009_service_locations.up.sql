CREATE TABLE service_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL REFERENCES organizations(id), customer_id UUID NOT NULL REFERENCES customers(id), name TEXT NOT NULL,
    address_line_1 TEXT NOT NULL, address_line_2 TEXT, city TEXT NOT NULL, postal_code TEXT, country CHAR(2) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90), longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    location GEOGRAPHY(POINT, 4326) NOT NULL, geofence_radius_meters INTEGER NOT NULL CHECK (geofence_radius_meters > 0), notes TEXT,
    active BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX service_locations_organization_id_idx ON service_locations (organization_id);
CREATE INDEX service_locations_customer_id_idx ON service_locations (customer_id);
CREATE INDEX service_locations_location_gist_idx ON service_locations USING GIST (location);
