-- Creates the 4 logical databases (one per consolidated service) on first
-- container init. Mounted into /docker-entrypoint-initdb.d by docker-compose.
-- Runs as the POSTGRES_USER superuser; databases are owned by that user so
-- each service connects with the same credentials but an isolated database.
--
-- Each service owns its schema and migrations independently (see
-- backend/services/<svc>/internal/infrastructure/repository/migrations). This
-- file only provisions the logical databases, never tables.

CREATE DATABASE identity_db;  -- Identity service (auth + orgs + admin)
CREATE DATABASE workspace_db; -- Workspace service (project + task + catalog + resources)
CREATE DATABASE agent_db;     -- Agent service (agent + settings)
CREATE DATABASE runner_db;    -- Executor service (runner, unchanged)
