# M1 — Accounts, persistence, and real link CRUD

Status: **in progress — production persistence remains**

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
- [x] CI green on the sign-in UI PR.
- [x] Merge the sign-in UI into `dev`.
- [x] Add a production SMTP sender with mandatory STARTTLS, bounded network deadlines, and sender/recipient validation.
- [x] Fail startup on mixed development-log/SMTP configuration and reject development code logging when secure cookies are enabled.

### Local authentication

Local authentication requires the managed schema plus a stable secret. One-time codes may be logged only when the local browser cookie is explicitly configured as non-secure:

```bash
npm run db:schema:apply
VUTAME_DATABASE_DSN='file:vutame.db' \
VUTAME_AUTH_SECRET='replace-with-at-least-32-random-bytes' \
VUTAME_AUTH_LOG_CODES=1 \
VUTAME_COOKIE_SECURE=0 \
npm run dev:api
```

`VUTAME_AUTH_LOG_CODES=1` is development-only. The server refuses to enable it while secure cookies are enabled.

### Hosted authentication email

Hosted environments use SMTP over mandatory STARTTLS:

```bash
VUTAME_DATABASE_DSN='...' \
VUTAME_AUTH_SECRET='replace-with-at-least-32-random-bytes' \
VUTAME_SMTP_ADDR='smtp.example.com:587' \
VUTAME_SMTP_USERNAME='smtp-user' \
VUTAME_SMTP_PASSWORD='smtp-password' \
VUTAME_AUTH_EMAIL_FROM='Vutame <login@vutame.com>' \
./vutame
```

The SMTP path is provider-neutral and can be configured with a hosted SMTP provider such as Amazon SES SMTP credentials. One-time codes are never written to application logs by this sender.

## Slice 3 — claiming and profile/link mutations

- [x] Claim handles transactionally with DB uniqueness as the final authority.
- [x] Add authenticated owned-profile read/update API.
- [x] Add link create/update/delete API.
- [x] Add link reorder API.
- [x] Add active/hidden toggle while keeping hidden links out of public profile responses.
- [x] Derive mutation ownership only from the authenticated session; browser requests never supply owner IDs.
- [x] Add store-level cross-account authorization coverage.
- [x] Add full HTTP lifecycle coverage for claim → profile edit → link CRUD/reorder/visibility.
- [x] Add the session-aware Solid creator dashboard at `/create`.
- [x] Add the Solid account/session screen at `/settings`.
- [x] CI green and merge the editor API and creator workspace into `dev`.

## Slice 4 — production persistence

- [ ] Add libSQL/Turso production adapter without changing domain interfaces.
- [ ] Add production Atlas plan/apply workflow and deployment documentation.
- [ ] Add integration tests against the production-style adapter.
- [ ] Remove transitional memory fallback from hosted configuration.

## M1 exit criteria

A new user can sign in, claim `vuta.me/@name`, edit their profile, add/reorder/hide/delete links, sign out, restart the server, and still see the same public profile. Unauthorized users cannot mutate another user's Vuta.

The local SQLite implementation now proves this product flow. **M1 remains open until Slice 4 proves the same behavior on the production persistence path and hosted configuration no longer depends on the transitional memory fallback.**
