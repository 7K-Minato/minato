# minato UI Feature Assessment

## Backend Capability Inventory

### Existing CRDs
| CRD | Scope | Status |
|-----|-------|--------|
| GameProfile | Cluster | ✅ Ready |
| GameServer | Namespace | ✅ Ready |
| GameServerFleet | Namespace | ✅ Ready |
| ActionExecution | Namespace | ✅ Ready |
| GameSnapshot | Namespace | ✅ Ready |

### Existing API Endpoints
```
GET    /healthz
GET    /readyz
GET    /api/v1/gameservers
GET    /api/v1/gameservers/{namespace}/{name}
POST   /api/v1/gameservers/{namespace}              [admin]
DELETE /api/v1/gameservers/{namespace}/{name}       [admin]
GET    /api/v1/gameservers/{namespace}/{name}/actions
POST   /api/v1/gameservers/{namespace}/{name}/actions/{action}  [operator+]
GET    /api/v1/gameservers/{namespace}/{name}/actions/{executionId}
GET    /api/v1/gameservers/{namespace}/{name}/snapshots
POST   /api/v1/gameservers/{namespace}/{name}/snapshots         [operator+]
GET    /api/v1/gameserverfleets
GET    /api/v1/gameserverfleets/{namespace}/{name}
GET    /api/v1/profiles
GET    /api/v1/profiles/{name}
GET    /api/v1/apikeys                                 [admin]
POST   /api/v1/apikeys                                 [admin]
DELETE /api/v1/apikeys/{keyId}                        [admin]
```

### Missing Endpoints (Future Backend Work)
- `PUT /api/v1/gameservers/{namespace}/{name}` — Update server (change env, storage)
- `PUT /api/v1/gameserverfleets/{namespace}/{name}` — Update fleet replicas
- `POST /api/v1/gameserverfleets/{namespace}` — Create fleet
- `DELETE /api/v1/gameserverfleets/{namespace}/{name}` — Delete fleet
- `GET /api/v1/gameservers/{namespace}/{name}/players` — Player list
- `WebSocket /api/v1/gameservers/{namespace}/{name}/console` — Console streaming
- `GET /api/v1/actionexecutions` — List action executions (audit log)
- `GET /api/v1/gamesnapshots` — List snapshots across namespace

---

## Feature Categories

### 1. Connection & Context Management

**Features:**
- Add control plane connection (URL, auth mode, credentials)
- Edit connection settings
- Remove connection
- Test connection (health check)
- Context switcher (header dropdown)
- Connection status indicator (online/offline)
- Context persistence (localStorage)

**RBAC:** N/A (local UI state)
**Backend Dependency:** `GET /healthz` for each context
**Complexity:** Low
**Priority:** **Must-have** — Required for multi-control-plane support

---

### 2. Global Dashboard (Front Page)

**Features:**
- Cluster overview cards (one per connected context)
  - Server count, player count, alerts
  - Connection status
- Global aggregations
  - Total servers across all contexts
  - Total players across all contexts
  - Servers by status (Running/Stopped/Error)
- Recent activity feed (from all contexts)
- Global player count chart (24h)
- Quick links to each context's server list

**RBAC:** Viewer (read-only)
**Backend Dependency:** `GET /api/v1/gameservers` for each context
**Complexity:** Medium
**Priority:** **Must-have** — Primary landing page

---

### 3. GameServer Management

#### 3.1 Server List
**Features:**
- List all servers in active context
- Filter by: namespace, profile, status, search by name
- Sort by: name, status, player count, age
- Card/list view toggle
- Status indicators (Running/Provisioning/Idle/Error)
- Player count (online/capacity)
- Quick actions: restart, stop, view console (if available)
- Bulk operations (delete multiple)

**RBAC:** Viewer (read), Admin (delete)
**Backend Dependency:** `GET /api/v1/gameservers`
**Complexity:** Medium
**Priority:** **Must-have**

#### 3.2 Server Detail
**Features:**
- Overview tab: status, endpoints, agent version, uptime
- Configuration tab: env vars, storage, lifecycle settings
- Events/Conditions tab: Kubernetes conditions, status transitions
- Actions tab: available actions, execute with parameters
- Snapshots tab: backup history, create new snapshot
- Players tab: online players (if agent supports it)
- Console tab: terminal emulator (if WebSocket available)
- Delete button with confirmation

**RBAC:** Viewer (read), Operator (actions, snapshots), Admin (delete)
**Backend Dependency:**
- `GET /api/v1/gameservers/{namespace}/{name}`
- `GET /api/v1/gameservers/{namespace}/{name}/actions`
- `GET /api/v1/gameservers/{namespace}/{name}/snapshots`
**Complexity:** High
**Priority:** **Must-have**

#### 3.3 Create Server
**Features:**
- Step 1: Select GameProfile (from available profiles)
- Step 2: Configure:
  - Name, namespace
  - Environment variables (from profile schema)
  - Storage size, storage class, snapshot restore
  - Lifecycle settings (idle timeout, auto-start)
- Step 3: Review & create
- Validation: name uniqueness, resource quotas

**RBAC:** Admin only
**Backend Dependency:**
- `GET /api/v1/profiles` (to list available)
- `POST /api/v1/gameservers/{namespace}`
**Complexity:** High
**Priority:** **Must-have**

---

### 4. GameServerFleet Management

#### 4.1 Fleet List
**Features:**
- List all fleets in active context
- Filter by: namespace, profile
- Show: replicas, ready count, profile
- Status: Updating/Stable/Error

**RBAC:** Viewer (read)
**Backend Dependency:** `GET /api/v1/gameserverfleets`
**Complexity:** Low
**Priority:** **Should-have**

#### 4.2 Fleet Detail
**Features:**
- Overview: replicas, ready count, update strategy
- Server list: managed servers with status
- Scale replicas (input field or +/- buttons)
- Rolling update status/progress
- Delete fleet

**RBAC:** Viewer (read), Admin (scale, delete)
**Backend Dependency:**
- `GET /api/v1/gameserverfleets/{namespace}/{name}`
- `GET /api/v1/gameservers` (filter by fleet label)
**Complexity:** Medium
**Priority:** **Should-have**

#### 4.3 Create Fleet
**Features:**
- Select profile
- Set replicas
- Configure template (env vars)
- Set update strategy (RollingUpdate/OnDelete)

**RBAC:** Admin only
**Backend Dependency:** Missing — no fleet creation endpoint
**Complexity:** Medium
**Priority:** **Nice-to-have** (blocked by backend)

---

### 5. GameProfile Browser

**Features:**
- Grid/list view of available profiles
- Search/filter by name, category
- Profile detail modal/page:
  - Display name, image, ports
  - Environment variable schema
  - Available actions catalog
  - Storage defaults
- No editing (profiles are cluster-scoped, managed by GitOps)

**RBAC:** Viewer (read-only)
**Backend Dependency:**
- `GET /api/v1/profiles`
- `GET /api/v1/profiles/{name}`
**Complexity:** Low
**Priority:** **Should-have** — Helps users understand available games

---

### 6. Snapshot Management

#### 6.1 Snapshot List
**Features:**
- List snapshots for a server
- Show: name, creation time, status (Pending/Ready/Failed), size
- Retention info (remaining lifetime)

**RBAC:** Viewer (read)
**Backend Dependency:**
- `GET /api/v1/gameservers/{namespace}/{name}/snapshots`
**Complexity:** Low
**Priority:** **Should-have**

#### 6.2 Create Snapshot
**Features:**
- One-shot snapshot (immediate)
- Scheduled snapshot setup:
  - Cron expression (with UI picker)
  - Retention: count + duration
- Execute button

**RBAC:** Operator+
**Backend Dependency:**
- `POST /api/v1/gameservers/{namespace}/{name}/snapshots`
**Complexity:** Medium
**Priority:** **Should-have**

#### 6.3 Restore from Snapshot
**Features:**
- Select snapshot from list
- Create new GameServer from snapshot
- Pre-fill form with snapshot data

**RBAC:** Admin
**Backend Dependency:**
- `POST /api/v1/gameservers/{namespace}` (with snapshotRef)
**Complexity:** Medium
**Priority:** **Should-have**

---

### 7. Action Execution & Audit

#### 7.1 Execute Actions
**Features:**
- Action catalog from profile
- Parameter form (generated from schema)
- Execute button with confirmation
- Progress indicator (polling execution status)
- Result display (success/failure + agent response)

**RBAC:** Operator+
**Backend Dependency:**
- `GET /api/v1/gameservers/{namespace}/{name}/actions`
- `POST /api/v1/gameservers/{namespace}/{name}/actions/{action}`
- `GET /api/v1/gameservers/{namespace}/{name}/actions/{executionId}`
**Complexity:** High
**Priority:** **Must-have** — Core functionality

#### 7.2 Action History / Audit Log
**Features:**
- List all action executions (namespace-scoped)
- Filter by: server, action name, status, date range
- Detail view: parameters, result, caller, timestamps
- Export to CSV/JSON

**RBAC:** Viewer (read)
**Backend Dependency:** Missing — no `GET /api/v1/actionexecutions` endpoint
**Complexity:** Medium
**Priority:** **Nice-to-have** (blocked by backend)

---

### 8. Console Streaming

**Features:**
- Terminal emulator (xterm.js)
- WebSocket connection to control plane
- Command input + output display
- Session management (connect/disconnect)
- Copy/paste support

**RBAC:** Operator+
**Backend Dependency:** Missing — WebSocket endpoint not implemented
**Complexity:** High
**Priority:** **Nice-to-have** (blocked by backend)

---

### 9. Player Management

**Features:**
- List online players (name, ID, join time)
- Player count chart (real-time)
- Kick/ban actions (game-dependent, requires agent support)

**RBAC:** Operator+
**Backend Dependency:** Missing — no player list endpoint
**Complexity:** Medium
**Priority:** **Nice-to-have** (blocked by backend)

---

### 10. API Key Management (Admin Only)

**Features:**
- List API keys
- Create new key (with role selection)
- Delete key
- Copy key to clipboard (one-time display)

**RBAC:** Admin only
**Backend Dependency:**
- `GET /api/v1/apikeys`
- `POST /api/v1/apikeys`
- `DELETE /api/v1/apikeys/{keyId}`
**Complexity:** Low
**Priority:** **Should-have**

---

### 11. Settings & Preferences

**Features:**
- Connections management (add/edit/remove control planes)
- Theme toggle (dark/light)
- Namespace filter (default namespace)
- Auto-refresh interval
- Notification preferences

**RBAC:** N/A (local UI state)
**Backend Dependency:** None
**Complexity:** Low
**Priority:** **Should-have**

---

## Feature Matrix by Role

| Feature | Viewer | Operator | Admin |
|---------|--------|----------|-------|
| Global Dashboard | ✅ | ✅ | ✅ |
| Server List | ✅ | ✅ | ✅ |
| Server Detail (read) | ✅ | ✅ | ✅ |
| Create Server | ❌ | ❌ | ✅ |
| Delete Server | ❌ | ❌ | ✅ |
| Execute Actions | ❌ | ✅ | ✅ |
| View Snapshots | ✅ | ✅ | ✅ |
| Create Snapshot | ❌ | ✅ | ✅ |
| Restore from Snapshot | ❌ | ❌ | ✅ |
| Fleet List | ✅ | ✅ | ✅ |
| Fleet Scale | ❌ | ❌ | ✅ |
| Profile Browser | ✅ | ✅ | ✅ |
| Console | ❌ | ✅ | ✅ |
| Player List | ✅ | ✅ | ✅ |
| API Key Management | ❌ | ❌ | ✅ |
| Audit Log | ✅ | ✅ | ✅ |
| Settings | ✅ | ✅ | ✅ |

---

## Priority Tiers

### Must-Have (MVP)
These are required for a functional v1 release:

1. **Connection Manager** — Multi-context is core to the design
2. **Global Dashboard** — Primary landing page
3. **Server List** — Basic server visibility
4. **Server Detail** — Status, config, conditions
5. **Create Server** — Wizard with profile selection
6. **Delete Server** — With confirmation
7. **Execute Actions** — Core differentiator
8. **Profile Browser** — Understand available games

### Should-Have (v1.1)
Important for production use:

9. **Fleet List & Detail** — For fleet operators
10. **Fleet Scale** — Adjust replicas
11. **Snapshot List** — Backup visibility
12. **Create Snapshot** — One-shot backups
13. **Restore from Snapshot** — Disaster recovery
14. **API Key Management** — For service accounts
15. **Settings** — User preferences

### Nice-to-Have (v1.2+)
Value-add features blocked by backend or lower priority:

16. **Console Streaming** — Requires WebSocket backend
17. **Player Management** — Requires player API endpoint
18. **Action History / Audit Log** — Requires listing endpoint
19. **Fleet Create/Delete** — Requires backend endpoints
20. **Bulk Operations** — Nice UX improvement
21. **Charts & Metrics** — Requires metrics aggregation
22. **Notifications** — Browser push notifications

---

## Backend Dependencies Summary

### Ready Now (No Backend Work)
- Connection management
- Global dashboard
- Server list, detail, create, delete
- Action execution
- Profile browser
- API key management
- Settings

### Requires Backend Addition
- **Fleet mutations** (`POST/PUT/DELETE` fleets)
- **Action history** (`GET /api/v1/actionexecutions`)
- **Console streaming** (WebSocket endpoint)
- **Player list** (`GET /api/v1/gameservers/{ns}/{name}/players`)
- **Server updates** (`PUT /api/v1/gameservers/{ns}/{name}`)

---

## Recommended Implementation Order

### Phase 1: Foundation (MVP)
- Connection manager
- Global dashboard
- Server list + detail
- Create/delete server
- Execute actions
- Profile browser

### Phase 2: Operations
- Fleet list + scale
- Snapshot management
- API keys
- Settings

### Phase 3: Advanced
- Console streaming (when backend ready)
- Player management (when backend ready)
- Audit log (when backend ready)
- Charts & metrics

---

*Status: Assessment Complete*
*Date: 2026-05-28*
