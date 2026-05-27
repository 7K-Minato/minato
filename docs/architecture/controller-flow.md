# Controller Architecture

This document describes the internal architecture and reconcile flow of each Minato controller.

## Overview

Minato uses the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) framework. Each controller watches one primary resource type and may own (create/manage) child resources.

```
┌─────────────────┐     watches     ┌──────────────────┐
│   GameServer    │────────────────▶│  GameServer      │
│   (primary)     │                 │  Reconciler      │
└─────────────────┘                 └──────────────────┘
                                           │
                                           │ owns
                                           ▼
                                    ┌──────────────────┐
                                     │  StatefulSet     │
                                     │  Service         │
                                     │  PVC             │
                                     └──────────────────┘
```

## GameServer Reconciler

### Reconcile Flow

The GameServer reconciler follows this 7-step flow:

1. **Fetch GameServer**: Get the GameServer object from the API server
2. **Handle Finalizer**: Add finalizer on creation, clean up resources on deletion
3. **Fetch Profile**: Get the referenced GameProfile
4. **Build Pod Spec**: Generate the game + agent container specification
5. **Reconcile StatefulSet**: Create or update the StatefulSet
6. **Reconcile Service**: Create or update the Service (game ports + agent gRPC)
7. **Reconcile PVC**: Create or update the PersistentVolumeClaim
8. **Update Status**: Set state (Provisioning/Running/Idle/Error) and conditions
10. **Health Check**: If running, check agent health via gRPC
11. **Idle Timeout**: If configured, check for idle timeout and scale down

### State Machine

```
  Created
     │
     ▼
Provisioning ◄────────────────┐
     │                        │
     │ StatefulSet Ready      │ Auto-start
     ▼                        │
  Running ──────idle─────▶  Idle
     │                        │
     │ players join           │
     └────────────────────────┘
```

### Key Decisions

- **One StatefulSet per GameServer**: Ensures stable network identity and persistent storage
- **Replicas always 1**: Game servers are singletons; scaling is done via GameServerFleet
- **Agent sidecar**: Every pod has exactly 2 containers (game + agent)
- **Finalizer-based cleanup**: Ensures PVC and other resources are deleted with the GameServer

### Idle Timeout Logic

```
Every reconcile (or RequeueAfter):
  1. Get player count from agent via gRPC
  2. If players > 0:
     - Update LastActivity = now
     - State = Running
  3. If players == 0:
     - If LastActivity is nil:
       - Set LastActivity = now
       - Requeue after idle timeout
     - If idle duration >= timeout:
       - Call agent PrepareShutdown
       - Scale StatefulSet to 0
       - State = Idle
```

## ActionExecution Reconciler

### Reconcile Flow

1. **Fetch ActionExecution**: Get the ActionExecution object
2. **Initialize State**: Set state to Pending if empty
3. **Validate Target**: Ensure the target GameServer exists
4. **Fetch Profile**: Get the GameProfile for action catalog
5. **Validate Action**: Ensure the action exists in the profile
6. **Validate Parameters**: Check required params are provided
7. **Check Concurrency**: Ensure no conflicting executions are running
8. **Dispatch**: Call agent via gRPC to execute the action
9. **Update Status**: Set state (Succeeded/Failed/Rejected) and timestamps

### Concurrency Control

Three concurrency modes:

- **allow**: Multiple instances of this action can run simultaneously
- **serialize**: Only one instance of this action can run at a time
- **exclusive**: No other action can run on this server while this one runs

### State Machine

```
Pending ──▶ Running ──▶ Succeeded
   │           │
   │           ├──▶ Failed
   │           │
   └──▶ Rejected ──▶ (no further transitions)
```

## GameServerFleet Reconciler

### Reconcile Flow

1. **Fetch Fleet**: Get the GameServerFleet object
2. **Handle Finalizer**: Add on creation, delete child GameServers on deletion
3. **List Children**: Find all GameServers owned by this fleet
4. **Scale Up**: Create missing GameServers
5. **Scale Down**: Delete excess GameServers (respecting update strategy)
6. **Update Status**: Update replica counts and conditions

### Update Strategies

**RollingUpdate** (default):
- When scaling down, delete oldest servers first (by creation timestamp)
- Ensures gradual replacement during updates

**OnDelete**:
- Never automatically delete excess servers
- User must manually delete GameServers
- Useful for controlled maintenance windows

## GameSnapshot Reconciler

### Reconcile Flow

1. **Fetch GameSnapshot**: Get the GameSnapshot object
2. **Handle Finalizer**: Add on creation
3. **Fetch GameServer**: Get the referenced GameServer
4. **Check Schedule**: If scheduled, check if it's time for a snapshot
5. **Create Snapshot**: Create a VolumeSnapshot of the GameServer's PVC
6. **Update Status**: Add snapshot entry to status
7. **Enforce Retention**: Delete old snapshots based on count/duration policy
8. **Requeue**: If scheduled, requeue after the interval

### Retention Policy

Two dimensions:
- **Count**: Keep only the N most recent snapshots
- **Duration**: Delete snapshots older than the specified duration

Both policies are applied independently. A snapshot is kept only if it satisfies both.

## ActionExecution Cleanup Task

A background task (not a traditional reconciler) that periodically cleans up old ActionExecutions:

- **Interval**: Every hour
- **Succeeded TTL**: 7 days
- **Failed/Rejected/TimedOut TTL**: 30 days
- **Running/Pending**: Never deleted

## External Resource Management

The operator manages only minato-native resources. External resources are managed separately:

- **ServiceMonitors**: Created by a separate Helm chart (not the operator)
- **Gateways / HTTPRoutes**: Created by a separate Helm chart (not the operator)
- **Ingress**: Created by a separate Helm chart (not the operator)

This separation keeps the operator focused and allows flexibility in how external resources are configured.

## Resource Ownership

All child resources have owner references pointing to their parent:

- StatefulSet, Service, PVC → GameServer
- GameServer → GameServerFleet
- VolumeSnapshot → GameSnapshot

This ensures proper garbage collection when parents are deleted.

## Error Handling

### Reconciler Error Strategy

1. **Transient errors** (API server unavailable): Return error to requeue
2. **Not found errors**: Log and return nil (object was deleted)
3. **Validation errors**: Update status with error condition, return nil
4. **Agent errors**: Log but don't fail reconcile (agent may be temporarily down)

### Status Updates

Status is updated independently of the spec. A failed status update is logged but doesn't fail the reconcile, preventing infinite loops.

## Testing Strategy

### Unit Tests

- Use `fake.NewClientBuilder()` for mocking the Kubernetes client
- Test each helper function independently
- Table-driven tests for multiple scenarios

### Integration Tests

- Use `envtest` for running a real (local) API server
- Test full reconcile loops
- Verify actual resource creation

### E2E Tests

- Use Kind for a real Kubernetes cluster
- Deploy the operator
- Create real GameServers
- Verify end-to-end functionality
