# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TuGo (Turbo Go) is a Go package that auto-generates REST APIs from PostgreSQL database schemas. Inspired by Directus but lighter, focusing on simplicity and flexibility. The package is designed for middleware-first integration (80%) with standalone mode as a secondary option (20%).

**Package:** `github.com/thienel/tugo`

## Repository Structure

- **Root (`/`)** - Main TuGo Go package (not yet implemented)
- **`tugo-docs/`** - React + Vite documentation app for the project architecture

## Development Commands

### Documentation App (tugo-docs/)

```bash
cd tugo-docs
npm install           # Install dependencies
npm run dev           # Start dev server
npm run build         # Build for production
npm run lint          # Run ESLint
npm run preview       # Preview production build
```

### Go Package (future)

The Go package will use:
- Go 1.21+
- sqlx for database operations
- Gin for internal routing
- PostgreSQL as the target database

## Architecture Summary

### Core Components

1. **Engine** - Orchestration layer, config management, HTTP lifecycle
2. **Schema Manager** - Table introspection with prefix-based discovery (`api_*` tables auto-expose)
3. **Collection Service** - CRUD operations with dynamic query building
4. **Auth Layer** - Multi-method auth (JWT, Cookie, TOTP)
5. **Permission System** - Policy-based access control with row-level filtering
6. **Storage Manager** - File storage abstraction (Local, MinIO, custom)

### API Namespaces

- `/api/v1/*` - Public API for CRUD operations
- `/api/admin/*` - Admin API for schema/permission management

### Key Design Patterns

- Prefix-based table discovery: Tables with `api_` prefix are auto-exposed
- System tables use `autoapi_` prefix (collections, permissions, roles, users, files)
- Dynamic routing based on collection names
- Policy-based permissions with Directus-style filters

## Documentation Files

- `tugo-docs/overall_docs.md` - Architecture and design document
- `tugo-docs/technical_specification.md` - Complete technical specification
