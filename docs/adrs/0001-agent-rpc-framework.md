# ADR 0001: Agent RPC Framework

## Status
Accepted

## Context
Minami needs a stable, public agent RPC contract that is used by the control plane to dispatch actions and by per-game agents to implement game-specific behavior. This contract is defined in protobuf and must be supported by the Go SDK and production-grade runtime.

## Decision
Use grpc-go (google.golang.org/grpc) for the agent RPC framework.

## Rationale
- grpc-go is the canonical Go gRPC implementation and aligns with Kubernetes ecosystem conventions.
- It keeps dependencies minimal and avoids introducing HTTP translation layers during bootstrap.
- It offers broad tooling support (Buf, protoc-gen-go, protoc-gen-go-grpc) and compatibility with other languages.
- It fits the internal service-to-service model expected for control plane to agent communication.

## Consequences
- The agent API is generated using protoc-gen-go and protoc-gen-go-grpc.
- If browser-friendly or HTTP/1.1 semantics become necessary later, we can add a gateway or adopt Connect in a future ADR.
