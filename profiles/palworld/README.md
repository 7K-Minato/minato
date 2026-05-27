# Palworld Profile

## Overview

This profile configures a Palworld dedicated server using the [thijsvanloef/palworld-server-docker](https://github.com/thijsvanloef/palworld-server-docker) Docker image.

## Game Image

- **Image**: `thijsvanloef/palworld-server-docker:latest`
- **Documentation**: https://github.com/thijsvanloef/palworld-server-docker/blob/main/README.md

## Agent

- **Image**: `ghcr.io/7k-group/minato-agent-palworld:v0.1.0`
- **RCON**: Enabled on port 25575
- **Version**: 0.1.0

## Supported Actions

| Action | Description | Parameters |
|--------|-------------|------------|
| `restart` | Restart the server | None |
| `save-world` | Save the world | None |
| `send-message` | Send message to players | `message` (required) |
| `kick-player` | Kick a player | `player` (required) |
| `ban-player` | Ban a player | `player` (required) |
| `list-players` | List online players | None |

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8211` | No | Game port |
| `PLAYERS` | `32` | No | Max players |
| `MULTITHREADING` | `true` | No | Enable multi-threading |
| `RCON_ENABLED` | `true` | No | Enable RCON |
| `ADMIN_PASSWORD` | `minato-admin` | No | Admin password |
| `SERVER_NAME` | `Minato Palworld Server` | No | Server name |

## Resources

- **Requests**: 2 CPU, 8Gi memory
- **Limits**: 8 CPU, 16Gi memory

## Storage

- **Mount Path**: `/palworld`
- **Default Size**: 50Gi

## Capabilities

- **Files**: Yes (filebrowser sidecar)
- **SFTP**: Yes
- **Backup**: Yes
- **Restore from Snapshot**: Yes

## Known Quirks

- Palworld server is memory intensive; minimum 8GB recommended
- Multi-threading significantly improves performance on multi-core systems
- World save files can grow large (5-10GB+ for busy servers)
