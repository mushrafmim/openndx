# Portal Backend

A secure Go-based REST API for managing data exchange workflows, including member management, schema submissions, and application processing.

## Overview

The Portal Backend provides REST APIs for the Admin Portal and Member Portal, handling authentication, authorization, and business logic for data exchange operations.

## Features

- **JWT Authentication** for a single API audience (`internal/authn`)
- **Role-Based Access Control (RBAC)** with granular permissions
- **PostgreSQL Database** with automatic schema management
- **Thread-Safe Caching** for optimal performance
- **OpenAPI Documentation** at `/openapi.yaml`
- **Comprehensive Health Monitoring**
- **Docker Support** for containerized deployment
- **Audit Logging** for compliance

## Quick Start

### Prerequisites

- Go 1.26+
- PostgreSQL 13+
- Docker (optional)

### 1. Environment Setup

> [!NOTE]
> The Portal Backend no longer auto-loads a `.env` file at startup — variables must come from the process environment. Copy `cmd/pb/.env.example` to `cmd/pb/.env`, update it, then source it manually before running the service (see step 2).

Copy `cmd/pb/.env.example` to `cmd/pb/.env` and update it:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USERNAME=postgres
DB_PASSWORD=your_password
DB_NAME=portal_backend
DB_SSLMODE=disable
RUN_MIGRATION=true

# JWT Authentication (Required)
IDP_BASE_URL=https://thunderid.example.com
# PB's API audience: its resource server identifier in the IdP
IDP_AUDIENCE=http://pb.openndx.local

# Policy Decision Point
PDP_SERVICE_URL=http://localhost:8082


# ThunderID management client (creates each application's OAuth2 client)
IDP_CLIENT_ID=PORTAL_BACKEND
IDP_CLIENT_SECRET=management_client_secret
IDP_SCOPE=system
# The System resource server identifier the `system` scope is requested for,
# and the role every new application is assigned (grants `getdata`).
IDP_TOKEN_RESOURCE=https://localhost:8090/mcp
IDP_DATA_CONSUMER_ROLE_ID=<role id>
```

`POST /api/v1/applications` creates the application's M2M client in the owning member's ThunderID
organization unit, assigns it `IDP_DATA_CONSUMER_ROLE_ID`, and returns its `idpClientSecret` once. The
local compose stack configures this with the `PORTAL_BACKEND` client from
`thunderid/bootstrap/application.yaml`.

### 2. Run the Service

```bash
# Load cmd/pb/.env into the shell environment (from the repo root)
set -a
source cmd/pb/.env
set +a

# Run the server
go run ./cmd/pb

# Or build and run
go build -o pb ./cmd/pb && ./pb
```

The service runs on port 8083 by default.

## Configuration

### Server Configuration

```bash
PORT=8083                         # Server port (default: 8083)
CORS_ALLOWED_ORIGINS=*            # CORS allowed origins
```

## API Endpoints

### Core Resources

- **Members** - `/api/v1/members` - User profile and membership management
- **Schemas** - `/api/v1/schemas` - Data schema definitions and management
- **Schema Submissions** - `/api/v1/schema-submissions` - Schema submission workflow
- **Applications** - `/api/v1/applications` - Application definitions
- **Application Submissions** - `/api/v1/application-submissions` - Application submission workflow

### System Endpoints

- **Health Check** - `/health` - System health and database status
- **API Documentation** - `/openapi.yaml` - OpenAPI specification

### Authentication & Authorization

**Supported Roles:**
- `OpenNDX_Admin` - Full system access
- `OpenNDX_Member` - Standard user access to own resources
- `OpenNDX_System` - System-level read access

**JWT Requirements** (validated by `internal/authn`):
- Issuer: `IDP_ISSUER`
- Audience: `IDP_AUDIENCE`, PB's resource server identifier. Any client's token for this audience is accepted; clients request it with `resource=` at login.
- Claims: `sub` and `client_id` are required; `roles` must include at least one role above. `ouId` and `email` are read when present.
- Validation: JWKS-based signature verification (`IDP_JWKS_URL`)

## Testing

### Run Tests

```bash
# Unit tests only (from the repo root)
go test ./cmd/pb/... ./internal/pb/...

# Integration tests with PostgreSQL
make test-postgres

# Tests with race detection
go test -race ./cmd/pb/... ./internal/pb/...

# Coverage report
go test -coverprofile=coverage.out ./cmd/pb/... ./internal/pb/...
go tool cover -html=coverage.out
```

### Test Database Setup

```bash
export TEST_DB_PASSWORD=test_password
make test-local
```

## Architecture

### Project Structure

```
cmd/pb/
└── main.go                 # Entry point: wires dependencies and registers every route

internal/pb/
├── application/           # Application context: applications, submissions, allow-lists
├── auth/                  # Roles/permissions & authorization middleware (tokens are validated by internal/authn)
├── database/              # GORM connection and migrations
├── idp/                   # Identity provider integrations
├── kernel/                # Types shared across domains (BaseModel, Status, Version)
├── member/                # Member context: model, DTOs, service, handlers
├── middleware/            # CORS & audit
├── policy/                # Client for the Policy Decision Point (PDP)
└── schema/                # Schema context: schemas, submissions, policy metadata

internal/utils/            # HTTP/server helpers shared across services
```

### Security Architecture

```
Request → CORS → JWT Validation → Authorization → Resource Access
    ↓        ↓           ↓              ↓             ↓
 Origin   Token      Role Check    Permission    Ownership
 Check    Verify     & Claims      Validation    Validation
```

### Database Schema

**Core Tables:**
- `members` - User profiles and membership information
- `schemas` - Data schema definitions with versioning
- `schema_submissions` - Schema submission workflow and status
- `applications` - Application templates and definitions
- `application_submissions` - Application submission workflow

**Features:**
- Auto-migration on startup
- Connection pooling with configurable limits
- Health monitoring with metrics
- Transaction support with timeouts

## Health Check

`GET /health` returns comprehensive system status:

```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "database": {
    "status": "connected",
    "open_connections": 5,
    "max_open_connections": 25
  }
}
```

## Docker

```bash
# Build image (from the repo root)
docker build -t portal-backend -f cmd/pb/Dockerfile .

# Run container
docker run -p 8083:8083 \
  -e DB_HOST=host.docker.internal \
  -e PDP_SERVICE_URL=http://host.docker.internal:8082 \
  --env-file cmd/pb/.env \
  portal-backend
```
