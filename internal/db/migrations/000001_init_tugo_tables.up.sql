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
CREATE INDEX idx_tugo_roles_name ON tugo_roles(name);

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
CREATE INDEX idx_tugo_users_username ON tugo_users(username);
CREATE INDEX idx_tugo_users_email ON tugo_users(email);
CREATE INDEX idx_tugo_users_role_id ON tugo_users(role_id);
CREATE INDEX idx_tugo_users_status ON tugo_users(status);

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
CREATE INDEX idx_tugo_sessions_user_id ON tugo_sessions(user_id);
CREATE INDEX idx_tugo_sessions_token ON tugo_sessions(token);
CREATE INDEX idx_tugo_sessions_refresh_token ON tugo_sessions(refresh_token);
CREATE INDEX idx_tugo_sessions_expires_at ON tugo_sessions(expires_at);

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
CREATE INDEX idx_autoapi_files_filename ON autoapi_files(filename);
CREATE INDEX idx_autoapi_files_path ON autoapi_files(path);
CREATE INDEX idx_autoapi_files_mimetype ON autoapi_files(mimetype);
CREATE INDEX idx_autoapi_files_storage_provider ON autoapi_files(storage_provider);
CREATE INDEX idx_autoapi_files_uploaded_by ON autoapi_files(uploaded_by);

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
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(role_id, collection, action)
);

-- Create indexes
CREATE INDEX idx_tugo_permissions_role_id ON tugo_permissions(role_id);
CREATE INDEX idx_tugo_permissions_collection ON tugo_permissions(collection);
CREATE INDEX idx_tugo_permissions_action ON tugo_permissions(action);

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
CREATE INDEX idx_tugo_audit_log_user_id ON tugo_audit_log(user_id);
CREATE INDEX idx_tugo_audit_log_action ON tugo_audit_log(action);
CREATE INDEX idx_tugo_audit_log_collection ON tugo_audit_log(collection);
CREATE INDEX idx_tugo_audit_log_created_at ON tugo_audit_log(created_at);

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
CREATE TRIGGER tugo_roles_updated_at
    BEFORE UPDATE ON tugo_roles
    FOR EACH ROW
    EXECUTE FUNCTION tugo_update_timestamp();

CREATE TRIGGER tugo_users_updated_at
    BEFORE UPDATE ON tugo_users
    FOR EACH ROW
    EXECUTE FUNCTION tugo_update_timestamp();

CREATE TRIGGER tugo_sessions_updated_at
    BEFORE UPDATE ON tugo_sessions
    FOR EACH ROW
    EXECUTE FUNCTION tugo_update_timestamp();

CREATE TRIGGER autoapi_files_updated_at
    BEFORE UPDATE ON autoapi_files
    FOR EACH ROW
    EXECUTE FUNCTION tugo_update_timestamp();

CREATE TRIGGER tugo_permissions_updated_at
    BEFORE UPDATE ON tugo_permissions
    FOR EACH ROW
    EXECUTE FUNCTION tugo_update_timestamp();
