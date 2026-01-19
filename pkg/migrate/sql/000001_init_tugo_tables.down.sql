-- TuGo System Tables Migration (Down)
-- Drops all system tables created by TuGo

-- Drop triggers first
DROP TRIGGER IF EXISTS tugo_permissions_updated_at ON tugo_permissions;
DROP TRIGGER IF EXISTS tugo_relationships_updated_at ON tugo_relationships;
DROP TRIGGER IF EXISTS tugo_fields_updated_at ON tugo_fields;
DROP TRIGGER IF EXISTS tugo_collections_updated_at ON tugo_collections;
DROP TRIGGER IF EXISTS tugo_files_updated_at ON tugo_files;
DROP TRIGGER IF EXISTS tugo_sessions_updated_at ON tugo_sessions;
DROP TRIGGER IF EXISTS tugo_users_updated_at ON tugo_users;
DROP TRIGGER IF EXISTS tugo_roles_updated_at ON tugo_roles;

-- Drop trigger function
DROP FUNCTION IF EXISTS tugo_update_timestamp();

-- Drop tables in reverse order of creation (respecting foreign key constraints)
DROP TABLE IF EXISTS tugo_audit_log;
DROP TABLE IF EXISTS tugo_permissions;
DROP TABLE IF EXISTS tugo_relationships;
DROP TABLE IF EXISTS tugo_fields;
DROP TABLE IF EXISTS tugo_collections;
DROP TABLE IF EXISTS tugo_files;
DROP TABLE IF EXISTS tugo_sessions;
DROP TABLE IF EXISTS tugo_users;
DROP TABLE IF EXISTS tugo_roles;

-- Note: We do not drop the uuid-ossp extension as it may be used by other tables
