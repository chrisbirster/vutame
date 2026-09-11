# M0 — Foundation

Status: **complete**

M0 exists to make later feature work predictable. It deliberately does not include authentication or durable user writes; those begin in M1.

## Acceptance criteria

- [x] `dev` exists as the permanent integration branch.
- [x] Feature branches target `dev`; releases promote `dev` to `main` and are tagged.
- [x] Go server uses `net/http`, graceful shutdown, and a single embedded SPA artifact.
- [x] Solid 2 + Solid Router + StyleX + TypeScript compile in CI.
- [x] Public routes include `/`, `/discover`, `/create`, and `/@handle`.
- [x] SPA fallback serves deep links from the embedded binary.
- [x] Public profile state is accessed through a `profile.Store` interface rather than package globals.
- [x] Profiles and links expose stable IDs independent of handles.
- [x] Public links have explicit position and active state.
- [x] Handles have documented validation/reserved-name rules.
- [x] Handle availability has an API endpoint.
- [x] Runtime metadata exposes marketing/profile origins.
- [x] Domain and HTTP behavior have Go tests.
- [x] CI is green on the M0 PR after these changes.
- [x] M0 PR is merged into `dev`.

## M0 API contract

```text
GET /api/v1/healthz
GET /api/v1/meta
GET /api/v1/discover
GET /api/v1/profiles/{handle}
GET /api/v1/handles/{handle}/availability
```

The memory store is intentionally temporary. M1 must preserve these public read contracts while swapping in durable persistence and adding authenticated mutations.

## Deferred to M1

- account registration/login;
- sessions and authorization;
- database schema and migrations/schema apply;
- actual handle claiming;
- profile/link mutation APIs;
- upload storage.

Keeping those out of M0 prevents auth/database decisions from being entangled with the basic web/runtime architecture.
