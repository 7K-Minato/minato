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

- `-s, --server`: Control plane API address (default: http://localhost:8080)
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
