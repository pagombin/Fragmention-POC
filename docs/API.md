# API reference

_This document is populated incrementally, one phase at a time. Each section
links to the canonical handler source._

All responses use the uniform envelope:

```json
{ "data": <payload> | null, "error": null | { "code": "...", "message": "...", "details": {...} } }
```

Every state-changing endpoint propagates `X-Request-ID`. If the client
supplies one, it is echoed; otherwise a v4 UUID is minted.

## Authentication

Phase 1 enables a static bearer token:

```
Authorization: Bearer <token>
```

The token must be ≥32 bytes; startup fails otherwise. Health, readiness, and
`/metrics` are exempt from auth.

## Phase-1 endpoints

| Method | Path                          | Description                             |
|--------|-------------------------------|-----------------------------------------|
| GET    | `/health`, `/api/v1/health`   | Liveness — static 200                   |
| GET    | `/ready`, `/api/v1/ready`     | Readiness — pings state store           |
| GET    | `/api/v1/version`             | Build identity                          |
| POST   | `/api/v1/admin/log-level`     | Change log level at runtime (body: `{"level":"debug"}`) |

More routes land in Phases 2, 7, and 8.
