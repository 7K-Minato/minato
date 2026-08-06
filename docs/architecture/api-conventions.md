# API conventions

These conventions apply to every HTTP API in the minato ecosystem: the control plane
REST API, minato cloud's public API, and any future service. The goal is that a client
written against one service needs zero new assumptions to talk to another.

## Source of truth

Every boundary has exactly one machine-readable contract; everything else is generated
from it or CI-checked against it.

| Boundary | Contract | Consumers |
|---|---|---|
| Control plane REST | `api/openapi.yaml` (OpenAPI 3.0, spec-first) | chi server interface (oapi-codegen), `sdk-go` client |
| Agent protocol | `proto/minato/agent/v1/agent.proto` (buf) | operator, control plane console, game agents |
| Kubernetes API | `api/operator/v1` CRD types (kubebuilder) | operator, control plane, kubectl |
| Console WebSocket | `docs/agent-developers/console-protocol.md` + `api/console.schema.json` | control plane, minato cloud proxy, frontends |
| minato cloud REST | `api/openapi.yaml` in the minato-cloud repo | dashboard (openapi-typescript) |

Rules:

- OpenAPI specs are edited **by hand first**; server types and clients are generated,
  never the other way around.
- A PR that changes API behavior must update the spec in the same commit.
- Generated code is checked in and CI fails if `make generate` produces a diff.
- Breaking changes are rejected by CI (`oasdiff` for OpenAPI, `buf breaking` for proto).

## Authentication

- Machine callers authenticate with an API key sent as
  `Authorization: Bearer minato_...`. The legacy `X-API-Key` header is accepted as an
  alias but new clients must use `Authorization: Bearer`.
- Human callers authenticate with an OIDC JWT sent as `Authorization: Bearer <jwt>`.
- Basic auth exists for local development only.
- Unauthenticated responses are `401` with a `WWW-Authenticate` header; authenticated
  but under-privileged responses are `403`.

## Errors

All non-2xx responses use a single JSON envelope:

```json
{ "error": "human-readable message" }
```

No `text/plain` errors, no mixed shapes. Clients may always decode the body as
`{"error": string}`.

## Versioning

- REST APIs are prefixed `/api/v1`. Additive changes (new optional fields, new
  endpoints) happen inside `v1`. Removing or renaming a field, or changing its type,
  requires `/api/v2`.
- CRD versions follow the Kubernetes lifecycle: `v1alpha1` (may change freely),
  `v1beta1` (additive only), `v1` (stable, deprecation window of one minor release
  before any removal).
- Deprecations are announced in the spec (`deprecated: true`) and in release notes at
  least one minor release before removal.

## Pagination and idempotency

- List endpoints return plain arrays today; when pagination is introduced it will use
  `limit`/`continue` query parameters mirroring the Kubernetes API, and will be an
  additive change.
- `POST` endpoints that create resources accept client-chosen names where possible so
  retries are safe; server-generated names embed a timestamp to avoid collisions.

## PR checklist for API changes

1. Spec updated (`api/openapi.yaml` / `.proto` / CRD types).
2. `make generate` run and output committed.
3. Consumers regenerated or pinned version bumped (sdk-go, minato-cloud client).
4. Conventions in this document still hold (auth, error envelope, versioning).
