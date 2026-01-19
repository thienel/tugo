-- TuGo System Tables Migration (Up)
-- Creates all required system tables for TuGo

-- Enable UUID extension if not exists
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- ROLES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description VARCHAR(500),
    is_system BOOLEAN DEFAULT FALSE,
    permissions JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_tugo_roles_name ON tugo_roles(name);

-- Insert default roles
INSERT INTO tugo_roles (id, name, description, is_system, permissions) VALUES
    ('00000000-0000-0000-0000-000000000001', 'admin', 'Full administrative access', TRUE, '{"*": ["create", "read", "update", "delete"]}'),
    ('00000000-0000-0000-0000-000000000002', 'user', 'Standard user access', TRUE, '{"*": ["read"]}'),
    ('00000000-0000-0000-0000-000000000003', 'guest', 'Limited guest access', TRUE, '{"*": []}')
ON CONFLICT (name) DO NOTHING;

-- ============================================================================
-- USERS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role_id UUID REFERENCES tugo_roles(id) ON DELETE SET NULL,
    status VARCHAR(50) DEFAULT 'active',
    totp_secret VARCHAR(255),
    totp_enabled BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}',
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_tugo_users_username ON tugo_users(username);
CREATE INDEX IF NOT EXISTS idx_tugo_users_email ON tugo_users(email);
CREATE INDEX IF NOT EXISTS idx_tugo_users_role_id ON tugo_users(role_id);
CREATE INDEX IF NOT EXISTS idx_tugo_users_status ON tugo_users(status);

-- ============================================================================
-- SESSIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES tugo_users(id) ON DELETE CASCADE,
    token VARCHAR(500) UNIQUE NOT NULL,
    refresh_token VARCHAR(500) UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    refresh_expires_at TIMESTAMP WITH TIME ZONE,
    user_agent VARCHAR(500),
    ip_address VARCHAR(45),
    is_revoked BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_tugo_sessions_user_id ON tugo_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_tugo_sessions_token ON tugo_sessions(token);
CREATE INDEX IF NOT EXISTS idx_tugo_sessions_refresh_token ON tugo_sessions(refresh_token);
CREATE INDEX IF NOT EXISTS idx_tugo_sessions_expires_at ON tugo_sessions(expires_at);

-- ============================================================================
-- FILES TABLE (Storage Metadata)
-- ============================================================================
CREATE TABLE IF NOT EXISTS autoapi_files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    filename VARCHAR(500) NOT NULL,
    original_filename VARCHAR(500),
    path VARCHAR(1000) NOT NULL,
    mimetype VARCHAR(255),
    size BIGINT,
    storage_provider VARCHAR(100) DEFAULT 'local',
    bucket VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    uploaded_by UUID REFERENCES tugo_users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_autoapi_files_filename ON autoapi_files(filename);
CREATE INDEX IF NOT EXISTS idx_autoapi_files_path ON autoapi_files(path);
CREATE INDEX IF NOT EXISTS idx_autoapi_files_mimetype ON autoapi_files(mimetype);
CREATE INDEX IF NOT EXISTS idx_autoapi_files_storage_provider ON autoapi_files(storage_provider);
CREATE INDEX IF NOT EXISTS idx_autoapi_files_uploaded_by ON autoapi_files(uploaded_by);

-- ============================================================================
-- COLLECTIONS TABLE (Schema Metadata)
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_collections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) UNIQUE NOT NULL,
    table_name VARCHAR(255) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    hidden BOOLEAN DEFAULT FALSE,
    singleton BOOLEAN DEFAULT FALSE,
    icon VARCHAR(100),
    note TEXT,
    display_template VARCHAR(500),
    archive_field VARCHAR(100),
    archive_value VARCHAR(100),
    sort_field VARCHAR(100),
    accountability VARCHAR(50) DEFAULT 'all',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tugo_collections_name ON tugo_collections(name);
CREATE INDEX IF NOT EXISTS idx_tugo_collections_table_name ON tugo_collections(table_name);

-- ============================================================================
-- FIELDS TABLE (Field Metadata)
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_fields (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    collection_id UUID NOT NULL REFERENCES tugo_collections(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    data_type VARCHAR(100) NOT NULL,
    postgres_type VARCHAR(100),
    is_nullable BOOLEAN DEFAULT TRUE,
    is_unique BOOLEAN DEFAULT FALSE,
    is_primary_key BOOLEAN DEFAULT FALSE,
    default_value TEXT,
    max_length INT,
    precision INT,
    scale INT,
    hidden BOOLEAN DEFAULT FALSE,
    readonly BOOLEAN DEFAULT FALSE,
    required BOOLEAN DEFAULT FALSE,
    sort INT DEFAULT 0,
    width VARCHAR(50) DEFAULT 'full',
    note TEXT,
    validation JSONB,
    display_options JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(collection_id, name)
);

CREATE INDEX IF NOT EXISTS idx_tugo_fields_collection_id ON tugo_fields(collection_id);
CREATE INDEX IF NOT EXISTS idx_tugo_fields_name ON tugo_fields(name);

-- ============================================================================
-- RELATIONSHIPS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_relationships (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    collection_id UUID NOT NULL REFERENCES tugo_collections(id) ON DELETE CASCADE,
    field_name VARCHAR(255) NOT NULL,
    related_collection_id UUID REFERENCES tugo_collections(id) ON DELETE CASCADE,
    related_collection VARCHAR(255),
    relationship_type VARCHAR(50) NOT NULL,
    junction_table VARCHAR(255),
    junction_field VARCHAR(255),
    one_field VARCHAR(255),
    many_field VARCHAR(255),
    one_deselect_action VARCHAR(50) DEFAULT 'nullify',
    sort_field VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tugo_relationships_collection_id ON tugo_relationships(collection_id);
CREATE INDEX IF NOT EXISTS idx_tugo_relationships_related_collection_id ON tugo_relationships(related_collection_id);
CREATE INDEX IF NOT EXISTS idx_tugo_relationships_type ON tugo_relationships(relationship_type);

-- ============================================================================
-- PERMISSIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role_id UUID NOT NULL REFERENCES tugo_roles(id) ON DELETE CASCADE,
    collection VARCHAR(255) NOT NULL,
    action VARCHAR(50) NOT NULL,
    filter JSONB DEFAULT '{}',
    field_permissions JSONB DEFAULT '{}',
    validation JSONB,
    presets JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(role_id, collection, action)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_tugo_permissions_role_id ON tugo_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_tugo_permissions_collection ON tugo_permissions(collection);
CREATE INDEX IF NOT EXISTS idx_tugo_permissions_action ON tugo_permissions(action);

-- ============================================================================
-- AUDIT LOG TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS tugo_audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES tugo_users(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    collection VARCHAR(255),
    item_id VARCHAR(255),
    changes JSONB,
    ip_address VARCHAR(45),
    user_agent VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_tugo_audit_log_user_id ON tugo_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_tugo_audit_log_action ON tugo_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_tugo_audit_log_collection ON tugo_audit_log(collection);
CREATE INDEX IF NOT EXISTS idx_tugo_audit_log_created_at ON tugo_audit_log(created_at);

-- ============================================================================
-- UPDATE TIMESTAMP TRIGGER FUNCTION
-- ============================================================================
CREATE OR REPLACE FUNCTION tugo_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to all tables with updated_at column
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_roles_updated_at') THEN
        CREATE TRIGGER tugo_roles_updated_at BEFORE UPDATE ON tugo_roles FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_users_updated_at') THEN
        CREATE TRIGGER tugo_users_updated_at BEFORE UPDATE ON tugo_users FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_sessions_updated_at') THEN
        CREATE TRIGGER tugo_sessions_updated_at BEFORE UPDATE ON tugo_sessions FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'autoapi_files_updated_at') THEN
        CREATE TRIGGER autoapi_files_updated_at BEFORE UPDATE ON autoapi_files FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_collections_updated_at') THEN
        CREATE TRIGGER tugo_collections_updated_at BEFORE UPDATE ON tugo_collections FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_fields_updated_at') THEN
        CREATE TRIGGER tugo_fields_updated_at BEFORE UPDATE ON tugo_fields FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_relationships_updated_at') THEN
        CREATE TRIGGER tugo_relationships_updated_at BEFORE UPDATE ON tugo_relationships FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tugo_permissions_updated_at') THEN
        CREATE TRIGGER tugo_permissions_updated_at BEFORE UPDATE ON tugo_permissions FOR EACH ROW EXECUTE FUNCTION tugo_update_timestamp();
    END IF;
END
$$;
