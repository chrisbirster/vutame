# M1 — Accounts, persistence, and real link CRUD

Status: **in progress**

Goal: let a real person sign in, claim a Vuta, and manage a durable profile.

## Slice 1 — managed persistence

- [x] Define the desired SQLite schema for `users`, `profiles`, `links`, and `sessions`.
- [x] Make Atlas the schema owner; normal application startup does not migrate.
- [x] Add a pure-Go SQLite driver so release binaries remain `CGO_ENABLED=0`.
- [x] Add a SQLite-backed implementation of the existing `profile.Store` interface.
- [x] Verify required schema tables at startup when SQLite is configured.
- [x] Preserve public filtering/order behavior for active links.
- [x] Add database-backed profile, link-order, handle-availability, and missing-schema tests.
- [ ] CI green on the persistence PR.
- [ ] Merge persistence slice into `dev`.

`VUTAME_DATABASE_DSN` enables the SQLite store. If it is absent during this transitional slice, the M0 memory seed remains available. Before M1 closes, durable storage will become the normal hosted path.

Local schema workflow:

```bash
atlas schema apply --env local --dry-run
atlas schema apply --env local --auto-approve
VUTAME_DATABASE_DSN='file:vutame.db' npm run dev:api
```

## Slice 2 — identity and sessions

- [ ] Add user creation/lookup repository operations.
- [ ] Add email verification challenge records with short expiry and one-time consumption.
- [ ] Send magic-link/code email through a provider abstraction.
- [ ] Create hashed server-side sessions.
- [ ] Add secure `HttpOnly`, `SameSite=Lax` session cookie handling.
- [ ] Add logout and expiry cleanup.
- [ ] Establish CSRF strategy for authenticated mutation routes.
- [ ] Add `/api/v1/auth/*` endpoints and corresponding Solid sign-in flow.

## Slice 3 — claiming and profile/link mutations

- [ ] Claim handles transactionally with DB uniqueness as the final authority.
- [ ] Add authenticated profile update API.
- [ ] Add link create/update/delete API.
- [ ] Add link reorder API.
- [ ] Add active/hidden toggle.
- [ ] Add settings/editor routes in the Solid SPA.
- [ ] Add ownership/authorization coverage.

## Slice 4 — production persistence

- [ ] Add libSQL/Turso production adapter without changing domain interfaces.
- [ ] Add production Atlas plan/apply workflow and deployment documentation.
- [ ] Add integration tests against the production-style adapter.
- [ ] Remove transitional memory fallback from hosted configuration.

## M1 exit criteria

A new user can sign in, claim `vuta.me/@name`, edit their profile, add/reorder/hide/delete links, sign out, restart the server, and still see the same public profile. Unauthorized users cannot mutate another user's Vuta.
