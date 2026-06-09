# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0](https://github.com/7K-Minato/minato/compare/v1.0.0...v1.1.0) (2026-06-09)


### Features

* update release workflow ([#7](https://github.com/7K-Minato/minato/issues/7)) ([83d9a51](https://github.com/7K-Minato/minato/commit/83d9a514c5f94a38a87a9a9bce3c9cd766dc78a0))

## 1.0.0 (2026-06-09)


### Features

* complete Phase 6 - comprehensive OSS documentation and architecture sync ([b4a1bff](https://github.com/7K-Minato/minato/commit/b4a1bff7c4e978d5bf98393bf704d231229a9c30))
* complete v1 release preparation ([b76d44b](https://github.com/7K-Minato/minato/commit/b76d44b13d4055569072b608e69cecd16d91958a))

## [Unreleased]

### Added

- Comprehensive test coverage (80%+ for core packages)
- Security headers middleware for control plane API
- Request size limiting (10MB max)
- Control plane hardening with security best practices
- CONTRIBUTING.md with development guidelines
- SECURITY.md with vulnerability reporting process
- Controller architecture documentation
- Installation and troubleshooting guides

### Fixed

- Fixed invalid Go version in go.mod (1.25.7 -> 1.23.4)
- Fixed hardcoded gRPC port in action execution controller
- Fixed GameSnapshot controller to create actual VolumeSnapshot objects
- Fixed GameProfile YAML structure (moved capabilities to correct location)
- Fixed missing Capabilities field in GameProfile types
- Fixed idle timeout requeue logic
- Fixed ServiceMonitor cleanup on GameServer deletion
- Fixed insecure WebSocket origin check (now configurable)
- Fixed Prometheus detection to re-check periodically
- Fixed GameServerFleet to respect update strategy (RollingUpdate/OnDelete)
- Fixed missing endpoints field in GameServerStatus
- Removed old minami.io CRD files
- Made CS2 and Palworld agents functional with RCON support
- Added RCON dialer implementations for Minecraft, Source, and Palworld

### Security

- Added security headers (X-Content-Type-Options, X-Frame-Options, CSP)
- Added request size limits
- Secured WebSocket origin checking
- Updated Docker base images to Go 1.23.4
- Added security policy and vulnerability reporting process

## [0.1.0] - 2026-05-27

### Added

- Initial release of Minato
- Core CRDs: GameProfile, GameServer, GameServerFleet, ActionExecution, GameSnapshot
- Operator with reconcilers for all CRDs
- Agent SDK with gRPC API
- Generic YAML-action agent
- Minecraft Paper agent with RCON support
- CS2 and Palworld agent stubs
- Control plane HTTP API
- WebSocket console streaming
- ServiceMonitor integration
- Helm chart for deployment
- Basic documentation

[Unreleased]: https://github.com/7k-group/minato/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/7k-group/minato/releases/tag/v0.1.0
