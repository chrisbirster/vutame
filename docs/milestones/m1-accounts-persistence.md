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
- [x] CI green on the persistence PR.
- [x] Merge persistence slice into `dev`.

## Slice 2 — identity and sessions

- [x] Add user creation/lookup as part of successful email verification.
- [x] Add email verification challenge records with 10-minute expiry and one-time transactional consumption.
- [x] Add an email sender abstraction with an explicitly local-only log sender.
- [x] Store challenge codes as HMAC-SHA256 hashes using a server secret.
- [x] Create random server-side sessions and store only token hashes.
- [x] Add secure `HttpOnly`, `SameSite=Lax` session cookie handling.
- [x] Add logout and session lookup.
- [x] Require JSON mutation requests as the initial CSRF boundary; no CORS mutation path is exposed.
- [x] Add `/api/v1/auth/code`, `/verify`, `/session`, and `/logout` endpoints.
- [x] Add service and HTTP tests for code/session lifecycle and cookie behavior.
- [x] CI green on the auth/session backend PR.
- [x] Merge auth/session backend into `dev`.
- [x] Add the Solid `/signin` email/code/session UI.
- [ ] CI green on the sign-in UI PR.
- [ ] Merge the sign-in UI into `dev`.
- [ ] Wire a production email provider (SES candidate).

Local authentication requires the managed schema plus a stable secret:

```bash
npm run db:schema:apply
VUTAME_DATABASE_DSN='file:vutame.db' \
VUTAME_AUTH_SECRET='replace-with-at-least-32-random-bytes' \
VUTAME_AUTH_LOG_CODES=1 \
VUTAME_COOKIE_SECURE=0 \
npm run dev:api
```

`VUTAME_AUTH_LOG_CODES=1` is development-only. Hosted environments must use a real sender and must not log one-time codes.

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
