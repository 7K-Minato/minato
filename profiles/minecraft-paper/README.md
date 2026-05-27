# Minecraft Paper Profile

## Overview

This profile configures a Minecraft Paper server using the [itzg/minecraft-server](https://github.com/itzg/docker-minecraft-server) Docker image.

## Game Image

- **Image**: `itzg/minecraft-server:latest`
- **Documentation**: https://github.com/itzg/docker-minecraft-server/blob/master/README.md

## Agent

- **Image**: `ghcr.io/7k-group/minato-agent-minecraft:v0.1.0`
- **RCON**: Enabled by default on port 25575
- **Version**: 0.1.0

## Supported Actions

| Action | Description | Parameters |
|--------|-------------|------------|
| `restart` | Gracefully restart the server | None |
| `save-world` | Save the game world | None |
| `send-message` | Broadcast a message | `message` (required) |
| `kick-player` | Kick a player | `player` (required), `reason` (optional) |
| `op-player` | Give operator status | `player` (required) |
| `deop-player` | Remove operator status | `player` (required) |
| `whitelist-add` | Add to whitelist | `player` (required) |
| `whitelist-remove` | Remove from whitelist | `player` (required) |

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `EULA` | `true` | Yes | Accept Minecraft EULA |
| `TYPE` | `PAPER` | No | Server type |
| `VERSION` | `1.20.4` | No | Minecraft version |
| `MEMORY` | `2G` | No | JVM heap size |
| `ENABLE_RCON` | `true` | No | Enable RCON |
| `RCON_PASSWORD` | `minato-rcon` | No | RCON password |

## Resources

- **Requests**: 500m CPU, 2Gi memory
- **Limits**: 2 CPU, 4Gi memory

## Storage

- **Mount Path**: `/data`
- **Default Size**: 20Gi

## Capabilities

- **Files**: Yes (filebrowser sidecar)
- **SFTP**: Yes
- **Backup**: Yes
- **Restore from Snapshot**: Yes

## Known Quirks

- Paper server requires at least 2GB RAM for stable operation
- First startup downloads Paper JAR which may take 1-2 minutes
- World generation on first start can be CPU intensive
