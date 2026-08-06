# Minato CLI (minato-ctl)

The official CLI for interacting with the Minato control plane.

## Installation

```bash
go install github.com/7k-minato/minato/cmd/minato-ctl@latest
```

## Configuration

The CLI connects to the control plane API. Configure the endpoint:

```bash
# Via flag
minato-ctl --server=http://minato-api.example.com server list

# Via environment variable
export MINATO_SERVER=http://minato-api.example.com
minato-ctl server list
```

## Commands

### Server Management

```bash
# List all game servers
minato-ctl server list

# List servers in a specific namespace
minato-ctl server list -n production

# Get server details
minato-ctl server get my-server

# Execute an action
minato-ctl server action my-server restart
minato-ctl server action my-server send-message message="Hello players!"
minato-ctl server action my-server kick-player player=badguy reason="Griefing"
```

### Console Access

```bash
# Open interactive console
minato-ctl console my-server

# The console streams logs and accepts commands:
> say Welcome to the server!
> list
> op admin_user
> save-all
```

### Fleet Management

```bash
# List fleets
minato-ctl fleet list

# Get fleet details
minato-ctl fleet get my-fleet
```

### Profile Management

```bash
# List available game profiles
minato-ctl profile list

# Get profile details
minato-ctl profile get minecraft-paper
```

### Snapshot Management

```bash
# List snapshots for a server
minato-ctl snapshot list my-server

# Create a new snapshot
minato-ctl snapshot create my-server
```

## Global Flags

- `-s, --server`: Control plane API address (default: <http://localhost:8080>)
- `-n, --namespace`: Default namespace (default: minato)
- `-h, --help`: Show help

## Examples

```bash
# Full workflow: create server, execute action, open console
minato-ctl server create -f my-server.yaml
minato-ctl server action my-server restart
minato-ctl console my-server

# Check all servers in a fleet
minato-ctl fleet get production-fleet
minato-ctl server list -n production
```

## Cloud Mode (minato-cloud SaaS)

`minato-ctl cloud` talks to a minato-cloud deployment (default
<http://localhost:8080>) instead of a control plane. It manages SaaS servers,
snapshots, actions, the catalog, plans/subscription and tenant API keys.

### Authentication

Two credential types, both sent as `Authorization: Bearer ...`:

```bash
# Tenant API key (mk_...), stored in ~/.config/minato/config.json (mode 0600)
minato-ctl cloud login --api-key mk_<prefix>_<secret>

# Keycloak session: paste an ID token from the cloud dashboard when prompted
minato-ctl cloud login

# Or via environment (takes precedence over stored credentials)
export MINATO_CLOUD_API_KEY=mk_<prefix>_<secret>
```

`minato-ctl cloud logout` removes stored credentials. `minato-ctl cloud
whoami` shows the URL, auth mode and your tenants.

The cloud URL resolves with precedence: `--url` flag > `MINATO_CLOUD_URL` >
stored config > `http://localhost:8080`.

### Tenants

All tenant-scoped commands accept `--tenant <id|slug|name>`; when you belong
to exactly one tenant it is used by default.

### Commands

```bash
minato-ctl cloud servers list
minato-ctl cloud servers get <id>
minato-ctl cloud servers create --name lobby --profile minecraft \
    [--tier small] [--region eu] [--storage 20Gi] [--env KEY=value,...]
minato-ctl cloud servers delete <id>

minato-ctl cloud snapshots list <server-id>
minato-ctl cloud snapshots create <server-id>

minato-ctl cloud actions list <server-id>
minato-ctl cloud actions run <server-id> <action> [--param k=v]...

minato-ctl cloud catalog          # games with tiers + regions
minato-ctl cloud plans
minato-ctl cloud subscription

minato-ctl cloud apikeys list
minato-ctl cloud apikeys create --name ci [--scope servers:read,snapshots]
minato-ctl cloud apikeys delete <id>   # the key is printed once at creation
```

All cloud commands print tables by default; pass `--json` for raw JSON.
Errors are mapped to actionable messages: 401 → run `cloud login`, 402 →
subscribe to a plan, 403 → check tenant role/quota/API key scopes.
