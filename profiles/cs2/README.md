# Counter-Strike 2 Profile

## Overview

This profile configures a Counter-Strike 2 dedicated server using the [joedwards32/cs2](https://github.com/joedwards32/CS2) Docker image.

## Game Image

- **Image**: `joedwards32/cs2:latest`
- **Documentation**: https://github.com/joedwards32/CS2/blob/main/README.md

## Agent

- **Image**: `ghcr.io/7k-group/minato-agent-cs2:v0.1.0`
- **RCON**: Source RCON on game port 27015
- **Version**: 0.1.0

## Supported Actions

| Action | Description | Parameters |
|--------|-------------|------------|
| `restart` | Restart the server | None |
| `change-map` | Change current map | `map` (required) |
| `send-message` | Send message to players | `message` (required) |
| `kick-player` | Kick a player | `player` (required), `reason` (optional) |
| `pause-match` | Pause current match | None |
| `swap-teams` | Swap teams | None |
| `set-warmup-time` | Set warmup duration | `seconds` (required) |
| `end-warmup` | End warmup | None |

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SRCDS_TOKEN` | - | Yes | Steam Game Server Login Token |
| `CS2_SERVERNAME` | `Minato CS2 Server` | No | Server name |
| `CS2_MAP` | `de_dust2` | No | Default map |
| `CS2_GAMEALIAS` | `casual` | No | Game mode alias |
| `CS2_MAXPLAYERS` | `64` | No | Max players |
| `CS2_RCONPW` | `minato-rcon` | No | RCON password |

## Resources

- **Requests**: 1 CPU, 4Gi memory
- **Limits**: 4 CPU, 8Gi memory

## Storage

- **Mount Path**: `/home/steam/cs2-dedicated`
- **Default Size**: 50Gi

## Capabilities

- **Files**: No
- **SFTP**: No
- **Backup**: No
- **Restore from Snapshot**: No

## Known Quirks

- CS2 dedicated server requires a Steam Game Server Login Token
- Server downloads game files on first start (~30GB)
- CPU requirements scale with player count and tickrate
