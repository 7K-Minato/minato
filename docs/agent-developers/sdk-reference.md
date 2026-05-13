# Agent SDK Reference

## server

- `server.Agent` interface: implement to expose gRPC methods.
- `server.Serve(agent, opts)` starts gRPC + metrics endpoints.
- `server.Options`:
  - `GRPCAddr` (default `:9876`)
  - `MetricsAddr` (default `:9090`)
  - `ShutdownGrace` (default `5s`)

## actions

- `actions.Catalog`: action definitions loaded from YAML/JSON.
- `actions.Execute(ctx, action, params, runtime)` runs steps.
- Supported step types: `rcon`, `exec`, `sleep`, `http`, `signal`.

## rcon

- `rcon.Client` interface: `Command(ctx, command)`.
- `rcon.Dialer` interface: `Dial(ctx, addr, password)`.
- `rcon.MockClient`, `rcon.MockDialer` for tests.

## metrics

- `metrics.PlayerCount(game, server)` returns canonical metric name.
- `metrics.ActionDuration(action, game)` returns canonical metric name.

## lifecycle

- `lifecycle.RunSteps(ctx, steps)` runs cleanup steps in order.

## testing

- `testing.NewFakeAgentEnv()` returns a fixture with a mock RCON client.
