-- PostgreSQL schema for the Policy Decision Point.
-- Apply to the PDP database before starting the service. Safe to re-run.

BEGIN;

-- Enum types. CREATE TYPE has no IF NOT EXISTS, so skip types that already exist.
DO $$ BEGIN
    CREATE TYPE source_enum AS ENUM ('primary', 'fallback');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE access_control_type_enum AS ENUM ('public', 'restricted');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE owner_enum AS ENUM ('citizen');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS policy_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schema_id VARCHAR(255) NOT NULL,
    field_name TEXT NOT NULL,
    display_name TEXT,
    description TEXT,
    source source_enum NOT NULL DEFAULT 'fallback',
    is_owner BOOLEAN NOT NULL DEFAULT false,
    access_control_type access_control_type_enum NOT NULL DEFAULT 'restricted',
    allow_list JSONB NOT NULL DEFAULT '{}',
    owner owner_enum,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- One policy record per field of a schema
CREATE UNIQUE INDEX IF NOT EXISTS idx_policy_metadata_schema_field
    ON policy_metadata(schema_id, field_name);

COMMIT;
