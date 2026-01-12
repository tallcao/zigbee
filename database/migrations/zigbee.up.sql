-- Device vendors table
CREATE TABLE vendors (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    code TEXT NOT NULL UNIQUE
);

-- Device categories table (e.g., Controller, Light, Sensor)
CREATE TABLE device_categories (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    code TEXT NOT NULL UNIQUE
);

-- Device models table
CREATE TABLE device_models (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_uuid UUID NOT NULL REFERENCES vendors(uuid),
    category_uuid UUID NOT NULL REFERENCES device_categories(uuid),
    name TEXT NOT NULL,
    model TEXT NOT NULL,
    description TEXT,
    properties JSONB,
    UNIQUE(vendor_uuid, model)
);



CREATE TABLE zigbee_devices (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    addr TEXT UNIQUE NOT NULL,
    device_uuid UUID NOT NULL REFERENCES devices(uuid)
);
