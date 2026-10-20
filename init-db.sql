-- Creates the databases required by the Exchange services.
-- Runs automatically on first Postgres container startup
-- (mounted into /docker-entrypoint-initdb.d).
--
-- The POSTGRES_USER (default: postgres) owns all databases.

-- Read the same CE database name used by the service's Compose configuration.
\set ce_database consent_engine
\getenv ce_database DB_NAME_CE
SELECT COALESCE(NULLIF(:'ce_database', ''), 'consent_engine') AS ce_database \gset

-- Idempotent creation: skip databases that already exist so the script is
-- safe to re-run manually (the postgres image only auto-runs it on first init).
SELECT 'CREATE DATABASE pdp' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pdp')\gexec
SELECT format('CREATE DATABASE %I', :'ce_database') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'ce_database')\gexec
SELECT 'CREATE DATABASE audit' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'audit')\gexec
SELECT 'CREATE DATABASE portal_backend' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'portal_backend')\gexec

-- Bootstrap CE's schema from its single source file (mounted by compose.yml).
\connect :"ce_database"
\i /opt/openndx/ce-schema.sql
