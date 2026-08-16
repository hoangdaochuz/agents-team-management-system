-- Creates the 10 logical databases (one per service) on first container init.
-- Mounted into /docker-entrypoint-initdb.d by docker-compose. Runs as the
-- POSTGRES_USER superuser; databases are owned by that user so each service
-- connects with the same credentials but an isolated database.
--
-- Each service owns its schema and migrations independently (see
-- backend/services/<svc>/internal/infrastructure/repository/migrations). This file only provisions
-- the logical databases, never tables.

-- Core runtime services (phases 3–9)
CREATE DATABASE project_db;
CREATE DATABASE task_db;
CREATE DATABASE agent_db;
CREATE DATABASE catalog_db;
CREATE DATABASE settings_db;
CREATE DATABASE runner_db;
-- Multi-tenant plane services (phases 10–13)
CREATE DATABASE auth_db;
CREATE DATABASE orgs_db;
CREATE DATABASE resources_db;
CREATE DATABASE admin_db;

