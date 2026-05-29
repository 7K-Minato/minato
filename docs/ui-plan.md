# minato UI Development Plan

## Overview

A web-based management interface for minato that communicates with the control plane API. Game-agnostic by design — all game-specific knowledge comes from the control plane, not hardcoded in the UI.

## Architecture

### Single Control Plane (Simple)

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   minato UI     │────▶│  Control Plane   │────▶│   Kubernetes    │
│   (React SPA)   │     │   (HTTP/gRPC)    │     │   API Server    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

### Multi Control Plane (Enterprise)

```
┌──────────────────────────────────────────────────────────────────┐
│                        minato UI                                  │
│                    (Connection Manager)                           │
├──────────────┬──────────────┬──────────────┬─────────────────────┤
│ Cluster A    │ Cluster B    │ Cluster C    │  (more...)          │
│ ┌──────────┐ │ ┌──────────┐ │ ┌──────────┐ │                     │
│ │ Control  │ │ │ Control  │ │ │ Control  │ │                     │
│ │ Plane A  │ │ │ Plane B  │ │ │ Plane C  │ │                     │
│ └────┬─────┘ │ └────┬─────┘ │ └────┬─────┘ │                     │
│      │       │      │       │      │       │                     │
│   K8s API    │   K8s API    │   K8s API    │                     │
└──────┴───────┴──────┴───────┴──────┴───────┴─────────────────────┘
```

The UI is designed to connect to **multiple control planes simultaneously** (like kubectl contexts). Each connection is isolated with its own auth credentials, cached data, and namespace filters. This is critical for:

- **Hosting providers** managing game servers across multiple customer clusters
- **Operators** with separate staging/production environments
- **Platform teams** overseeing multiple regions or data centers

**Design Principles:**
- Game-agnostic: No game-specific code. Profiles define available actions, ports, env vars.
- Real-time: WebSocket/SSE for live updates (player counts, server status, console output).
- Responsive: Works on desktop and tablet (mobile as read-only).
- Themeable: Dark mode default (gaming aesthetic), light mode available.
- Multi-tenant: Support for multiple control plane connections with isolated state.

---

## Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| **Framework** | React 18 + TypeScript | Industry standard, excellent type safety |
| **Build Tool** | Vite | Fast dev server, modern bundling |
| **Styling** | Tailwind CSS + shadcn/ui | Rapid development, accessible components |
| **State Management** | TanStack Query (React Query) | Server state caching, background refetching |
| **Routing** | TanStack Router | Type-safe routing, excellent dev UX |
| **Real-time** | Native WebSocket | Simple, no dependencies |
| **Charts** | Recharts | Lightweight, React-native |
| **Forms** | React Hook Form + Zod | Type-safe validation |
| **Icons** | Lucide React | Clean, consistent icon set |

### Next.js vs React + Vite

We evaluated Next.js but chose **React + Vite** for the following reasons:

| Concern | Next.js | React + Vite |
|---------|---------|--------------|
| **SSR/SSG** | Built-in | Not needed — admin dashboards don't need SEO |
| **API Routes** | Built-in | Not needed — we proxy directly to control planes |
| **Multi-control-plane** | Complex (server-side state isolation) | Natural (client-side connection manager) |
| **Build Speed** | Slower (server bundles) | Instant HMR, faster builds |
| **Bundle Size** | Larger (framework overhead) | Smaller (only what we use) |
| **Deployment** | Requires Node.js runtime | Static files (nginx/CDN) |
| **Complexity** | Higher (app router, server components) | Lower (pure client-side SPA) |

**Why we chose Vite for now:**

1. **No SEO benefit:** This is an internal admin tool, not a public-facing site
2. **Server complexity:** Next.js API routes would add an unnecessary middle layer between the UI and control plane
3. **State isolation:** Multi-control-plane support is simpler when all state lives client-side (TanStack Query caches per-connection)
4. **Deployment simplicity:** A React SPA builds to static files deployable on any CDN or nginx container
5. **Faster iteration:** Vite HMR is significantly faster than Next.js dev server for this use case

### Long-Term Architecture Recommendation

**A pure SPA is NOT enough for a long-term enterprise platform.** Here's why and what to do about it:

**SPA Limitations (you'll hit these within 6-12 months):**

| Problem | Impact | When It Happens |
|---------|--------|-----------------|
| **No SSR/SSG** | Can't have public status pages, server listings, or player leaderboards indexed by Google | When you want public visibility |
| **Client-side secrets** | API keys, OAuth client secrets visible in browser bundle | When adding integrations (Discord, Stripe) |
| **No server-side caching** | Every user independently hits control planes; no shared caching | At 50+ concurrent users |
| **Bundle size** | Entire app downloaded upfront; admin features bloat the bundle for public users | As features grow |
| **Auth vulnerabilities** | JWT in localStorage is XSS-vulnerable; no httpOnly cookies possible | Security audit |
| **SEO impossible** | No social previews, no search indexing | Marketing wants visibility |

**Recommended Evolution:**

```
Phase 1 (Now):     React SPA (Vite) ─────────────────────► Admin Dashboard
                                      │
Phase 2 (3-6mo):   Add Next.js App Router ───────────────► Public Pages + API
                                      │
Phase 3 (6-12mo):  Extract Shared Package ───────────────► Mobile App, CLI GUI
                                      │
Phase 4 (12mo+):   BFF or Serverless ────────────────────► Caching, Integrations
```

**Phase 2: Next.js App Router (The Right Long-Term Choice)**

Next.js App Router is the correct long-term framework because it **subsumes** Vite (everything Vite does, Next.js can do) while giving you future capabilities:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Next.js App Router                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │  Server Comp.   │  │  Client Comp.   │  │  API Routes     │  │
│  │  (SSR/SSG)      │  │  (SPA behavior) │  │  (BFF layer)    │  │
│  │                 │  │                 │  │                 │  │
│  │ • Status pages  │  │ • Admin dashboard│  │ • Auth proxy    │  │
│  │ • Public listing│  │ • Multi-context │  │ • Caching       │  │
│  │ • SEO content   │  │ • Real-time     │  │ • Webhooks      │  │
│  │ • Marketing     │  │ • Console       │  │ • Integrations  │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Why Next.js App Router specifically:**

1. **Server Components** render on the server with zero client-side JavaScript — perfect for public pages that don't need interactivity
2. **Client Components** work exactly like React SPA components — perfect for the admin dashboard
3. **API Routes** give you a Backend-for-Frontend without deploying a separate service
4. **Streaming SSR** lets you show loading states while data fetches — better UX than blank screens
5. **Edge Runtime** can run API routes at the edge (Cloudflare, Vercel) for low-latency auth and caching

**Multi-Control-Plane with Next.js:**

The admin dashboard stays client-side (Client Components), so multi-context management is identical to the SPA approach:

```typescript
// app/dashboard/page.tsx — Server Component (SSR for public data)
export default async function DashboardPage() {
  // Server-side: fetch public data, render SEO-friendly HTML
  return <DashboardClient />;
}

// components/dashboard/DashboardClient.tsx — Client Component
'use client';
export function DashboardClient() {
  const { activeContext } = useContextManager(); // Same as SPA
  const { data: servers } = useGameServers();    // TanStack Query
  // ... identical to Vite SPA
}
```

**Migration Path from Vite to Next.js:**

When you're ready to migrate (Phase 2), the process is straightforward:

1. **Move `src/` to `app/`** — Rename `main.tsx` to `layout.tsx`, add `'use client'` directives
2. **Replace Vite Router with Next.js Router** — File-based routing instead of TanStack Router
3. **Extract API calls to Server Components** — Public pages become Server Components
4. **Add API routes** — `app/api/` for auth proxy, caching, integrations
5. **Keep admin dashboard unchanged** — It's already client-side, works as-is

**Estimated effort:** 2-3 days for a working migration, 1 week for full optimization.

**My Recommendation:**

- **Start with Vite SPA** (Milestones 1-3) — Ship fast, validate the product
- **Migrate to Next.js App Router** (before Milestone 4 or during) — When you need public pages or API routes
- **Keep the admin dashboard as Client Components** — Never migrate the admin UI to Server Components; it doesn't benefit from SSR

This gives you the best of both worlds: rapid initial development with a clear upgrade path to enterprise-grade architecture.

---

## API Integration

### Multi Control Plane Support

The UI can manage multiple minato installations. Each connection is called a **Context** (similar to kubectl contexts):

```typescript
interface ControlPlaneContext {
  id: string;                    // uuid
  name: string;                  // display name (e.g., "Production EU", "Staging")
  url: string;                   // control plane URL
  auth: AuthConfig;
  // Optional metadata
  color?: string;                // UI color coding
  icon?: string;                 // emoji or icon name
}

interface AuthConfig {
  mode: 'none' | 'basic' | 'oidc' | 'apikey';
  // Basic auth
  username?: string;
  password?: string;
  // OIDC
  oidcIssuer?: string;
  clientId?: string;
  // API Key
  apiKey?: string;
}
```

**Context Switching:**
- User can add multiple contexts via Settings → Connections
- Active context shown in header dropdown
- Data cached per-context (TanStack Query `queryKey` includes context ID)
- Simultaneous views: Can open multiple browser tabs with different contexts

**API Client Factory:**

```typescript
// Each context gets its own axios instance with interceptors
function createApiClient(context: ControlPlaneContext) {
  const client = axios.create({ baseURL: context.url });
  
  // Attach auth headers based on context.auth
  client.interceptors.request.use((config) => {
    switch (context.auth.mode) {
      case 'basic':
        config.headers.Authorization = `Basic ${btoa(`${context.auth.username}:${context.auth.password}`)}`;
        break;
      case 'apikey':
        config.headers['X-API-Key'] = context.auth.apiKey;
        break;
      case 'oidc':
        config.headers.Authorization = `Bearer ${getOIDCToken(context)}`;
        break;
    }
    return config;
  });
  
  return client;
}
```

**TanStack Query Integration:**

```typescript
// Queries are scoped to the active context
function useGameServers() {
  const { activeContext } = useContextManager();
  
  return useQuery({
    queryKey: ['gameservers', activeContext.id],  // <-- context ID in query key
    queryFn: () => activeContext.api.listGameServers(),
    refetchInterval: 5000,
  });
}
```

### Authentication

Each context has independent auth. The UI supports all auth modes the control plane supports.

### API Client

Auto-generated from OpenAPI spec (or hand-written TypeScript interfaces):

```typescript
// Example generated types
interface GameServer {
  name: string;
  namespace: string;
  spec: GameServerSpec;
  status: GameServerStatus;
}

interface GameServerStatus {
  state: 'Provisioning' | 'Running' | 'Idle' | 'Stopped' | 'Error';
  players: number;
  playerCapacity: number;
  agentVersion: string;
  endpoints: Endpoint[];
  conditions: Condition[];
}
```

### Error Handling

- Network errors: Retry with exponential backoff (TanStack Query default)
- 401/403: Redirect to login
- 404: Show "Not Found" page
- 500: Show error toast with request ID

---

## Milestones

### Milestone 1: Foundation (Week 1-2)

**Goal:** Project scaffolding, multi-context auth, basic navigation, server list.

**Features:**
- [ ] Vite + React + TypeScript project setup
- [ ] Tailwind + shadcn/ui component library
- [ ] TanStack Router with route definitions
- [ ] TanStack Query setup with multi-context API client factory
- [ ] Connection Manager: Add/remove/switch control plane contexts
- [ ] Auth mode detection per context (call `/healthz` to discover auth requirements)
- [ ] Login page (supports all auth modes, per-context)
- [ ] Basic layout (sidebar navigation + context switcher in header)
- [ ] Navigation: Dashboard, Servers, Fleets, Profiles, Snapshots
- [ ] Context persistence (localStorage)

**Pages:**
- `/login` — Auth selection and login form (context-aware)
- `/` — **Global Dashboard** — Overview of all connected control planes
- `/servers` — GameServer list (from active context)
- `/settings/connections` — Manage control plane connections

**API Endpoints Needed:**
- `GET /healthz` (public, used for auth discovery)
- `GET /api/v1/gameservers`
- `GET /api/v1/profiles`

### Global Dashboard (Front Page)

The root route `/` serves as the **mission control** for all connected clusters:

```
┌──────────────────────────────────────────────────────────────────────┐
│  minato                                    [Search] [Theme] [User]   │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Connected Clusters                              [+ Add Cluster]     │
│                                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │ 🟢 Prod EU   │  │ 🟢 Prod US   │  │ 🟡 Staging   │              │
│  │              │  │              │  │              │              │
│  │ 12 servers   │  │ 8 servers    │  │ 3 servers    │              │
│  │ 142 players  │  │ 89 players   │  │ 0 players    │              │
│  │ 2 alerts     │  │ 0 alerts     │  │ 1 alert      │              │
│  │              │  │              │  │              │              │
│  │ [View]       │  │ [View]       │  │ [View]       │              │
│  └──────────────┘  └──────────────┘  └──────────────┘              │
│                                                                      │
│  Recent Activity                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ [Prod EU] minecraft-smp-1 restarted by admin       2m ago   │   │
│  │ [Prod US] cs2-competitive scaled to 5 replicas     5m ago   │   │
│  │ [Staging] Snapshot created for palworld-test       1h ago   │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  Global Player Count (24h)                                           │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  200 ┤                                              ╭─╮      │   │
│  │  150 ┤                                          ╭──╯ ╰──╮   │   │
│  │  100 ┤                              ╭────╮──────╯        │   │   │
│  │   50 ┤          ╭──╮    ╭────╮──────╯    ╰─────╮         │   │   │
│  │    0 ┼────╮─────╯  ╰────╯    ╰─────────────────╯─────────│   │   │
│  │      00:00  06:00  12:00  18:00  00:00                   │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

**Dashboard Cards (per context):**
- **Connection status:** Online/offline indicator
- **Server summary:** Total, running, stopped, error counts
- **Player count:** Current online / total capacity
- **Active alerts:** Servers in error state or unreachable agents
- **Quick actions:** "View" button switches to that context

**Global aggregations:**
- Total servers across all contexts
- Total players across all contexts
- Recent activity feed (from all contexts, sorted by time)
- Global player count chart (24h trend)

**Implementation:**
- Parallel TanStack Query calls to all online contexts
- `Promise.allSettled()` to handle partial failures (one cluster down shouldn't break the dashboard)
- Background refetching every 10s for live updates

**Deliverable:** Working dev server, can add multiple control plane connections, switch between them, view global dashboard, and view server lists from each.

---

### Milestone 2: Server Management (Week 3-4)

**Goal:** Full GameServer CRUD, detail view, basic actions.

**Features:**
- [ ] GameServer list page with filtering/search
- [ ] GameServer detail page (status, endpoints, players, conditions)
- [ ] Create GameServer wizard (select profile → configure env → set storage)
- [ ] Delete GameServer with confirmation
- [ ] Execute actions (restart, save, etc.) with progress toast
- [ ] Real-time status updates via polling (every 5s when viewing server)

**Pages:**
- `/servers` — List with filters (by profile, status, namespace)
- `/servers/:namespace/:name` — Detail view
- `/servers/create` — Create wizard

**API Endpoints Needed:**
- `GET /api/v1/gameservers/:namespace/:name`
- `POST /api/v1/gameservers/:namespace`
- `DELETE /api/v1/gameservers/:namespace/:name`
- `GET /api/v1/gameservers/:namespace/:name/actions`
- `POST /api/v1/gameservers/:namespace/:name/actions/:action`

**Deliverable:** Can create, view, manage, and delete game servers. Execute basic actions.

---

### Milestone 3: Fleets & Profiles (Week 5-6)

**Goal:** Fleet management, profile browser, scaling operations.

**Features:**
- [ ] GameServerFleet list page
- [ ] Fleet detail page (replicas, ready count, update strategy)
- [ ] Scale fleet replicas (up/down)
- [ ] Fleet rolling update status/progress
- [ ] GameProfile browser (view all configured games)
- [ ] Profile detail (ports, env vars, actions catalog)

**Pages:**
- `/fleets` — Fleet list
- `/fleets/:namespace/:name` — Fleet detail with replica management
- `/profiles` — Profile browser (card grid)
- `/profiles/:name` — Profile detail

**API Endpoints Needed:**
- `GET /api/v1/gameserverfleets`
- `GET /api/v1/gameserverfleets/:namespace/:name`
- `GET /api/v1/profiles`
- `GET /api/v1/profiles/:name`

**Deliverable:** Can manage fleets and browse available game profiles.

---

### Milestone 4: Snapshots & Backups (Week 7)

**Goal:** Snapshot management, restore from snapshot.

**Features:**
- [ ] Snapshot list per server
- [ ] Create snapshot (one-shot)
- [ ] Configure scheduled snapshots (cron UI)
- [ ] View snapshot status (pending/ready/failed)
- [ ] Restore: Create new GameServer from snapshot
- [ ] Retention policy display

**Pages:**
- `/servers/:namespace/:name/snapshots` — Snapshot history
- `/servers/:namespace/:name/snapshots/create` — Create snapshot form
- `/servers/:namespace/:name/snapshots/restore` — Restore wizard

**API Endpoints Needed:**
- `GET /api/v1/gameservers/:namespace/:name/snapshots`
- `POST /api/v1/gameservers/:namespace/:name/snapshots`

**Deliverable:** Full backup/restore workflow.

---

### Milestone 5: Console & Monitoring (Week 8-9)

**Goal:** Real-time console streaming, player monitoring, metrics dashboard.

**Features:**
- [ ] Console streaming (xterm.js in browser, WebSocket to control plane)
- [ ] Player list with kick/ban actions (game-dependent)
- [ ] Basic metrics dashboard (player count over time, server uptime)
- [ ] Alert conditions (server down, high latency, etc.)

**Pages:**
- `/servers/:namespace/:name/console` — Terminal emulator
- `/servers/:namespace/:name/players` — Player management
- `/dashboard` — Real-time overview with charts

**API Endpoints Needed:**
- `GET /api/v1/gameservers/:namespace/:name/console` (WebSocket upgrade)
- `GET /api/v1/gameservers/:namespace/:name/players`

**Deliverable:** Real-time monitoring and console access.

---

### Milestone 6: Polish & Production (Week 10)

**Goal:** Production readiness, theming, mobile support.

**Features:**
- [ ] Dark/light theme toggle
- [ ] Mobile-responsive layout (read-optimized)
- [ ] Keyboard shortcuts (Ctrl+K command palette)
- [ ] Offline indicator
- [ ] Error boundaries with retry
- [ ] Accessibility audit (WCAG 2.1 AA)
- [ ] Docker image for UI (nginx static serve)
- [ ] Helm chart update (add UI deployment)

**Deliverable:** Production-ready UI, deployable via Helm.

---

## Component Specifications

### Layout

```
┌─────────────────────────────────────────────────────────────┐
│  [Logo]  minato  [▼ Prod EU]       [Search] [Theme] [User]  │
├──────────┬──────────────────────────────────────────────────┤
│          │                                                    │
│ Dashboard│  Main Content Area                                │
│ Servers  │                                                    │
│ Fleets   │                                                    │
│ Profiles │                                                    │
│ Snapshots│                                                    │
│          │                                                    │
├──────────┤                                                    │
│ Settings │                                                    │
│ Logout   │                                                    │
└──────────┴──────────────────────────────────────────────────┘
```

**Context Switcher (Header):**
- Dropdown showing active context name + color indicator
- Click to switch between configured contexts
- "+ Add Context" button for new connections
- Each context shows connection status (● online, ● offline)

**Sidebar:** Collapsible on mobile, icons + labels on desktop.

### GameServer List Item

```
┌─────────────────────────────────────────────────────────────┐
│ [status dot] minecraft-smp-1           Running ● 12/20      │
│ minecraft-paper • default • 2d uptime    [Restart] [Stop]   │
└─────────────────────────────────────────────────────────────┘
```

Status dots: 🟢 Running, 🟡 Idle, 🔴 Error, ⚪ Provisioning

### Action Execution Toast

```
┌─────────────────────────────┐
│ Executing: save-world       │
│ [=====>    ] 50%            │
│ [Cancel]           [View]   │
└─────────────────────────────┘
```

Shows progress for long-running actions. Polls execution status.

---

## State Management

### Server State (TanStack Query)

```typescript
// Queries
const { data: servers } = useQuery({
  queryKey: ['gameservers'],
  queryFn: () => api.listGameServers(),
  refetchInterval: 5000, // Poll every 5s
});

// Mutations
const createServer = useMutation({
  mutationFn: api.createGameServer,
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ['gameservers'] });
  },
});
```

### Local State (React Context)

- **ContextManager:** Active control plane context, list of configured contexts
- **Auth state:** Per-context auth tokens/credentials
- **Theme preference:** Dark/light mode
- **Sidebar collapsed state:** Boolean
- **Active namespace filter:** Per-context namespace filter
- **Connection health:** Online/offline status per context

---

## Real-time Updates

### Polling Strategy

| Data | Interval | Priority |
|------|----------|----------|
| Server list | 5s | High |
| Server detail | 3s | High |
| Fleet status | 10s | Medium |
| Snapshot status | 15s | Low |
| Player count | 5s | High |

### WebSocket (Future)

When control plane supports WebSocket pub/sub:
- Subscribe to server status changes
- Subscribe to action execution completion
- Subscribe to fleet scaling events

Fallback to polling until then.

---

## Authentication Flow

```
1. UI loads → GET /healthz (no auth)
2. Control plane responds with auth mode in headers:
   X-Auth-Mode: basic
   X-OIDC-Issuer: https://auth.example.com
3. UI shows appropriate login form
4. User submits credentials
5. UI stores token/cookie
6. All subsequent requests include Authorization header
```

**Session Management:**
- JWT tokens: Store in memory (secure)
- API keys: Store in localStorage (user must regenerate if lost)
- Basic auth: Store in memory, prompt on 401
- OIDC: Standard OAuth2 PKCE flow

---

## Error Handling

### Global Error Boundary

Catches React rendering errors, shows fallback UI with reload button.

### API Error Toast

```typescript
// Example error toast
{
  title: "Failed to create server",
  description: "Profile 'minecraft-paper' not found",
  action: <Button>Retry</Button>,
  variant: "destructive"
}
```

### Offline Handling

- Detect `navigator.onLine`
- Show banner: "You are offline. Changes will sync when connection is restored."
- Queue mutations, replay when online

---

## File Structure

```
ui/
├── public/
│   └── favicon.ico
├── src/
│   ├── api/
│   │   ├── client.ts          # Axios/fetch wrapper
│   │   ├── types.ts           # Generated from OpenAPI
│   │   └── hooks.ts           # TanStack Query hooks
│   ├── components/
│   │   ├── ui/                # shadcn components
│   │   ├── layout/
│   │   │   ├── Sidebar.tsx
│   │   │   ├── Header.tsx
│   │   │   └── Shell.tsx
│   │   └── server/
│   │       ├── ServerCard.tsx
│   │       ├── ServerList.tsx
│   │       ├── ServerStatus.tsx
│   │       └── ActionButton.tsx
│   ├── features/
│   │   ├── auth/
│   │   │   ├── LoginPage.tsx
│   │   │   ├── AuthProvider.tsx
│   │   │   └── useAuth.ts
│   │   ├── servers/
│   │   │   ├── ServerListPage.tsx
│   │   │   ├── ServerDetailPage.tsx
│   │   │   └── CreateServerPage.tsx
│   │   ├── fleets/
│   │   ├── profiles/
│   │   ├── snapshots/
│   │   └── console/
│   ├── hooks/
│   │   └── useWebSocket.ts
│   ├── lib/
│   │   ├── utils.ts           # cn() helper
│   │   └── constants.ts
│   ├── routes/
│   │   └── __root.tsx         # TanStack Router root
│   ├── stores/
│   │   └── authStore.ts
│   ├── types/
│   │   └── index.ts
│   ├── App.tsx
│   └── main.tsx
├── index.html
├── package.json
├── tsconfig.json
├── tailwind.config.js
└── vite.config.ts
```

---

## Development Workflow

```bash
# Start UI dev server
cd ui
npm install
npm run dev

# The UI proxies API requests to the control plane
# Configure in vite.config.ts:
# server: { proxy: { '/api': 'http://localhost:8080' } }
```

**Hot Reload:** Vite provides instant HMR for React components.

**Mock Mode:** `npm run dev:mock` starts UI with MSW (Mock Service Worker) for offline development.

---

## Deployment

### Docker

```dockerfile
# ui/Dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

### Helm

Add to `deploy/helm/minato/templates/ui-deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minato-ui
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minato-ui
  template:
    metadata:
      labels:
        app: minato-ui
    spec:
      containers:
      - name: ui
        image: ghcr.io/7k-group/minato-ui:v0.1.0
        ports:
        - containerPort: 80
        env:
        - name: API_URL
          value: "http://minato-controlplane:8080"
```

---

## Extensibility

The React + Vite SPA architecture is intentionally simple to allow additive enhancement without rewrites.

### Caching

**Client-side (available now):**
- **TanStack Query cache:** Already handles server state caching with configurable stale times
- **localStorage/sessionStorage:** For user preferences, auth tokens, context configuration
- **IndexedDB:** For offline support with large datasets (use `idb` or `dexie` libraries)
- **Service Worker:** Convert to PWA for offline-first capability. Cache static assets + API responses.

**Server-side (add when needed):**
If you need shared caching across users or aggressive data reduction:

```
┌──────────┐     ┌──────────────┐     ┌──────────────────┐
│  UI SPA  │────▶│  API Gateway │────▶│  Control Plane   │
│          │     │  (optional)  │     │                  │
└──────────┘     └──────────────┘     └──────────────────┘
                        │
                        ▼
                   ┌──────────┐
                   │  Redis   │
                   │  Cache   │
                   └──────────┘
```

Add a thin **Backend-for-Frontend (BFF)** when:
- Multiple UI clients need the same cached data
- You want to reduce API calls (aggregate endpoints)
- You need server-side rendering for specific pages
- **No need to rewrite the UI** — just change the `baseURL` from control plane to BFF

### 3rd Party Integrations

**Client-side (safe):**
- OAuth flows (Discord, GitHub login)
- Analytics (Plausible, PostHog)
- Error tracking (Sentry)
- Chat widgets (Intercom, Crisp)

**Server-side required (add BFF or serverless):**
- Webhook receivers (GitHub, Stripe, Discord bots)
- API keys that must be secret (payment processors, email services)
- Scheduled jobs (cron-based reports)

**Serverless option (no BFF needed):**
Deploy serverless functions alongside the SPA:
```
ui/
├── src/              # React SPA
├── functions/        # Vercel/Netlify/Cloudflare functions
│   ├── discord-webhook.ts
│   ├── stripe-webhook.ts
│   └── daily-report.ts
└── dist/             # Built SPA
```

This gives you server capabilities without managing a Node.js backend.

### When to Migrate from SPA

The current architecture scales surprisingly far. Consider migration only when:

| Trigger | Solution |
|---------|----------|
| Need SEO for public pages | Add Next.js for marketing pages only, keep admin as SPA |
| Complex server-side caching | Add BFF (Express/Fastify) or serverless functions |
| Real-time collaboration | Add WebSocket server (Socket.io, PartyKit) |
| Native mobile app | Use React Native or Capacitor, reuse API client |

**Migration path:** The UI is modular. You can extract `src/api/`, `src/features/`, and `src/components/` into a shared package, then consume from Next.js, React Native, or another framework without rewriting business logic.

## Testing Strategy

| Type | Tool | Coverage |
|------|------|----------|
| Unit | Vitest | Components, hooks, utilities |
| Integration | React Testing Library | Page flows, form submissions |
| E2E | Playwright | Critical paths: login → create server → execute action |
| Visual | Storybook | Component isolation, design review |

---

## Future Enhancements (Post-v1)

- **Multi-cluster support:** Manage multiple Kubernetes clusters from one UI
- **Custom dashboards:** User-configurable metrics panels
- **Audit log viewer:** Browse ActionExecution history with filtering
- **RBAC management:** Visual role assignment (admin/operator/viewer)
- **Mobile app:** React Native companion for on-the-go monitoring
- **Plugin system:** Custom panels per game profile

---

## Decision Log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Framework | React + Vite | SPA is sufficient, Next.js adds complexity |
| Styling | Tailwind + shadcn | Rapid development, accessible defaults |
| State | TanStack Query | Built-in caching, background refetching |
| Router | TanStack Router | Type-safe, excellent DX |
| Charts | Recharts | Lightweight, React-native |
| Real-time | Polling (now), WebSocket (future) | Polling works today, WebSocket later |
| Auth | Support all control plane modes | Consistent with backend flexibility |
| Multi-control-plane | Client-side context manager | TanStack Query naturally isolates per-context cache |
| Next.js vs Vite | Vite | No SSR needed, simpler deployment, faster builds |

---

*Status: Planning*
*Last Updated: 2026-05-28*
