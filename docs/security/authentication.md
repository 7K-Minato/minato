# Control Plane Authentication & Authorization

## Overview

The minato control plane supports multiple authentication mechanisms simultaneously. Requests are authenticated via a chain of providers, and authorization is enforced through role-based access control (RBAC).

---

## Authentication Modes

All modes can be enabled simultaneously. The system tries each provider in order:

```
1. API Key (X-API-Key header) → fastest, for services
2. Bearer Token (Authorization header) → OIDC
3. Basic Auth (Authorization header)
```

### Mode: `none` (Development)

No authentication. All requests are treated as anonymous admin.

```bash
AUTH_MODE=none
```

**Use case:** Local development, testing.

---

### Mode: `basic` (Simple Setups)

Single shared username/password with bcrypt hashing.

```bash
AUTH_MODE=basic
AUTH_BASIC_ENABLED=true
AUTH_BASIC_USER=admin
AUTH_BASIC_PASSWORD=secret123        # Plaintext (hashed on startup)
AUTH_BASIC_ROLE=admin                # viewer | operator | admin
```

**Use case:** Small teams, internal tools, simple deployments.

**Security note:** Password is hashed with bcrypt cost 10 on first startup. Use `AUTH_BASIC_PASSWORD_HASH` to provide a pre-hashed value.

---

### Mode: `apikey` (Service-to-Service)

API keys are **generated dynamically by authenticated users** and stored in Kubernetes Secrets. They are **not** configured via environment variables.

```bash
# Enable API key support
AUTH_MODE=basic,apikey
AUTH_BASIC_ENABLED=true
AUTH_BASIC_USER=admin
AUTH_BASIC_PASSWORD=secret123
AUTH_APIKEY_ENABLED=true
```

**Important:** API keys are created via the API, not loaded from env vars.

#### Generating an API Key

```bash
# 1. Authenticate as a user (basic or OIDC)
curl -u admin:secret123 \
  http://minato-controlplane:8080/api/v1/gameservers

# 2. Generate an API key (inherits your role or specify one)
curl -u admin:secret123 \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"name": "ci-cd-pipeline", "role": "operator"}' \
  http://minato-controlplane:8080/api/v1/apikeys

# Response:
{
  "name": "ci-cd-pipeline",
  "role": "operator",
  "createdAt": "2026-05-27T20:55:00Z",
  "key": "minato_a1b2c3d4e5f6...",
  "warning": "This key will never be shown again. Store it securely."
}
```

**Security:** The key value is displayed **exactly once** during creation. It cannot be retrieved later.

#### Using an API Key

```bash
curl -H "X-API-Key: minato_a1b2c3d4e5f6..." \
  http://minato-controlplane:8080/api/v1/gameservers
```

#### Listing API Keys

```bash
curl -u admin:secret123 \
  http://minato-controlplane:8080/api/v1/apikeys

# Returns (without actual key values):
[
  {
    "name": "ci-cd-pipeline",
    "userId": "admin",
    "username": "admin",
    "role": "operator",
    "createdAt": "2026-05-27T20:55:00Z"
  }
]
```

#### Revoking an API Key

```bash
curl -u admin:secret123 \
  -X DELETE \
  http://minato-controlplane:8080/api/v1/apikeys/ci-cd-pipeline
```

#### Storage

API keys are stored as Kubernetes Secrets in the control plane namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: minato-apikey-ci-cd-pipeline
  namespace: minato-system
  labels:
    minato.io/apikey: "true"
    minato.io/created-by: admin
type: Opaque
data:
  key: bWluYXRvX2ExYjJjM2Q0ZTVmNi4uLg==      # base64 encoded key
  metadata: eyJuYW1lIjoiY2ktY2QtcGlwZWxpbmUiLCJyb2xlIjoib3BlcmF0b3IiLi4ufQ==
```

**Use case:** CI/CD pipelines, automated tools, service accounts.

---

### Mode: `oidc` (Enterprise SSO)

Generic OIDC-compatible provider (Keycloak, Auth0, Okta, etc.).

```bash
AUTH_MODE=oidc
AUTH_OIDC_ENABLED=true
AUTH_OIDC_ISSUER_URL=https://auth.example.com
AUTH_OIDC_CLIENT_ID=minato-controlplane
AUTH_OIDC_ROLE_CLAIM=groups           # JWT claim to read roles from
```

**Role mapping:** OIDC claim values are mapped to minato roles via RBAC config.

**Use case:** Enterprise deployments, SSO integration.

**Current limitation:** Simplified JWKS implementation. For production, implement full RSA key parsing.

---

## Combining Modes

Enable multiple modes simultaneously:

```bash
AUTH_MODE=basic,apikey,oidc

# Basic auth for human operators
AUTH_BASIC_ENABLED=true
AUTH_BASIC_USER=admin
AUTH_BASIC_PASSWORD=secret123

# API keys for automation (generated via API, not configured)
AUTH_APIKEY_ENABLED=true

# OIDC for enterprise users
AUTH_OIDC_ENABLED=true
AUTH_OIDC_ISSUER_URL=https://auth.example.com
```

---

## Role-Based Access Control (RBAC)

### Predefined Roles

| Role | Permissions |
|------|-------------|
| **viewer** | Read-only access to all resources (GET /gameservers, /profiles, /fleets, etc.) |
| **operator** | Viewer + execute actions, create snapshots, access console |
| **admin** | Operator + create/delete servers, manage API keys |

### Permission Model

Roles use a permission-based system:

```go
type Permission struct {
    Resource   string   // gameservers, profiles, actions, etc.
    Verbs      []string // get, list, create, delete, execute
    Namespaces []string // optional: restrict to specific namespaces
}
```

Roles can extend other roles (inheritance):
- `operator` extends `viewer`
- `admin` extends `operator`

### Route-Level Enforcement

```go
// Public (no auth required)
GET /healthz
GET /readyz

// Viewer+ can read
GET /api/v1/gameservers
GET /api/v1/profiles

// Operator+ can execute actions
POST /api/v1/gameservers/{ns}/{name}/actions/{action}

// Admin only
POST /api/v1/gameservers/{namespace}
DELETE /api/v1/gameservers/{namespace}/{name}
POST /api/v1/apikeys                    # Generate API keys
DELETE /api/v1/apikeys/{name}           # Revoke API keys
```

---

## Audit Logging

Every request (except health endpoints) is logged as structured JSON:

```json
{
  "timestamp": "2026-05-27T16:00:00Z",
  "level": "audit",
  "user": "alice",
  "role": "admin",
  "method": "POST",
  "path": "/api/v1/gameservers/default/minecraft-smp-1/actions/save-world",
  "clientIP": "10.0.0.15",
  "userAgent": "minato-ctl/v1.0",
  "result": "success"
}
```

**Fields:**
- `timestamp` — ISO 8601 timestamp
- `user` — Authenticated username
- `role` — User's role
- `method` — HTTP method
- `path` — Request path
- `clientIP` — Source IP address
- `userAgent` — Client user agent
- `result` — `success` or `failure`

**Output:** JSON to stdout (use log aggregation to collect)

---

## API Usage Examples

### With Basic Auth

```bash
curl -u admin:secret123 \
  http://minato-controlplane:8080/api/v1/gameservers
```

### With API Key

```bash
curl -H "X-API-Key: minato_a1b2c3d4e5f6..." \
  http://minato-controlplane:8080/api/v1/gameservers
```

### With OIDC Bearer Token

```bash
curl -H "Authorization: Bearer <oidc-token>" \
  http://minato-controlplane:8080/api/v1/gameservers
```

---

## Implementation Details

### Architecture

```
┌─────────────────────────────────────────────┐
│           HTTP Request                       │
│                                              │
│   Headers: Authorization / X-API-Key         │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│         Auth Middleware                      │
│                                              │
│   Skip if /healthz or /readyz                │
│   Try providers in order:                    │
│   1. API Key (reads from K8s Secrets)        │
│   2. Bearer Token (OIDC)                     │
│   3. Basic Auth                              │
│                                              │
│   Attach User to context                     │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│         Audit Middleware                     │
│                                              │
│   Log request with user info                 │
│   Capture response status                    │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│         RBAC Middleware                      │
│                                              │
│   Check role has permission for              │
│   resource + verb                            │
│                                              │
│   Return 403 if forbidden                    │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│         API Handler                          │
│                                              │
│   User available via context                 │
│   Perform action, write response             │
└─────────────────────────────────────────────┘
```

### Files

- `internal/controlplane/auth/auth.go` — Core interfaces, User, context helpers
- `internal/controlplane/auth/none.go` — No-op provider
- `internal/controlplane/auth/basic.go` — Basic auth with bcrypt
- `internal/controlplane/auth/apikey.go` — API key validation (reads from K8s Secrets)
- `internal/controlplane/auth/storage.go` — API key storage using K8s Secrets
- `internal/controlplane/auth/oidc.go` — OIDC JWT validation
- `internal/controlplane/rbac/rbac.go` — Role definitions, RequireRole middleware
- `internal/controlplane/audit/audit.go` — Structured audit logging

---

## Security Recommendations

### For Development
```bash
AUTH_MODE=none
```

### For Small Teams
```bash
AUTH_MODE=basic,apikey
AUTH_BASIC_USER=admin
AUTH_BASIC_PASSWORD=<strong-password>
AUTH_APIKEY_ENABLED=true
```

**Note:** API keys are generated via the API after authenticating with basic auth.

### For Production / Enterprise
```bash
AUTH_MODE=oidc,apikey
AUTH_OIDC_ISSUER_URL=https://auth.company.com
AUTH_OIDC_CLIENT_ID=minato-production
AUTH_APIKEY_ENABLED=true
```

**Additional measures:**
- Use HTTPS/TLS for all traffic
- Rotate API keys regularly
- Implement rate limiting (not yet implemented)
- Enable network policies to restrict control plane access
- Audit log all API key generation and usage

---

## Future Enhancements

- [ ] **OIDC JWKS** — Full RSA key parsing with key rotation support
- [ ] **Custom Roles** — Config file (`/etc/minato/rbac.yaml`) for custom role definitions
- [ ] **Rate Limiting** — Per-user/per-key request throttling
- [ ] **Session Management** — Cookie-based sessions for browser users
- [ ] **Audit Export** — Send audit logs to external SIEM (Splunk, ELK, etc.)

---

*Status: Implemented*
*Date: 2026-05-27*
*Authors: minato Core Team*
