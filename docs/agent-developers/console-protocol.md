# Console Streaming Protocol

The Minato console streaming protocol provides real-time bidirectional communication between clients and game server agents via WebSocket.

## WebSocket Endpoint

```
WS /api/v1/gameservers/{namespace}/{name}/console
```

### Authentication

The WebSocket connection uses the same authentication as the REST API. Pass authentication tokens via query parameters:

```
ws://control-plane/api/v1/gameservers/minato/server-1/console?namespace=minato&token=...
```

## Message Protocol

All messages are JSON-encoded.

### Client → Server Messages

#### RCON Command

```json
{
  "type": "rcon",
  "data": "say Hello World"
}
```

#### Ping (keepalive)

```json
{
  "type": "ping",
  "data": "hello"
}
```

### Server → Client Messages

#### Log Line

```json
{
  "type": "log",
  "ts": 1684156800,
  "line": "[12:00:00 INFO]: Player joined the game"
}
```

#### RCON Response

```json
{
  "type": "rcon-response",
  "data": "Server response here"
}
```

#### Status Update

```json
{
  "type": "status",
  "data": "running"
}
```

#### Error

```json
{
  "type": "error",
  "data": "Failed to execute command"
}
```

## Agent Implementation

Agents implement the `Console` gRPC method and translate between gRPC streams and the WebSocket protocol.

### Backpressure

If the client cannot keep up with log throughput, the server buffers up to 1000 lines, then drops old lines with a warning message.

### Reconnection

If the agent's stream dies (pod restart), the control plane returns a status message indicating disconnection. The client should reconnect automatically.

### Example Agent Implementation

```go
func (a *myAgent) Console(stream agentv1.Agent_ConsoleServer) error {
    // Forward RCON commands to game server
    // Stream game server logs back to client
    // Handle ping messages
}
```

## CLI Usage

```bash
# Open interactive console
minato-ctl console my-server

# The console streams logs in real-time
# Type RCON commands and press Enter
> say Hello World
> list
> stop
```
