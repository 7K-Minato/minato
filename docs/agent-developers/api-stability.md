# Agent API Stability

Minami's agent API is versioned by protobuf package name (e.g., `minami.agent.v1`).

## Versioning policy

- Minor releases add fields/methods in a backward-compatible way.
- Breaking changes require a new package version (`v2`) and a new SDK module path.
- Deprecated fields are supported for at least one minor Minami release after announcement.

## Deprecation timeline

1. Announce deprecation in release notes.
2. Mark fields/methods as deprecated in proto and SDK.
3. Remove only in the next major package version.
