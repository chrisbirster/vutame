# Vutame architecture

## Product model

Vutame is a link-in-bio product that grows into a creator discovery and portable social identity network.

The two domains intentionally have different jobs:

- `vutame.com` — marketing, onboarding, account management, profile editing, discovery, following, analytics, billing, and network features.
- `vuta.me/@handle` — the compact public identity URL users put in bios, QR codes, email signatures, and social profiles.

Both domains initially terminate at the same Go service. The application exposes the configured origins through `GET /api/v1/meta` so clients never need to hard-code deployment-specific hosts.

## Runtime architecture

```text
Browser
  │
  ├─ vutame.com ─────── marketing / app / discovery
  └─ vuta.me/@handle ─ public profile surface
              │
              ▼
       Go 1.26 net/http
       ┌──────────────────────────────┐
       │ /api/v1/*      JSON API      │
       │ /*             SPA fallback  │
       └──────────────────────────────┘
              │
       ┌──────┴─────────────┐
       │ profile.Store      │ public reads
       │ profile.Editor     │ authenticated writes
       │ auth.Store         │ identity + sessions
       └──────┬─────────────┘
              │ database/sql
       ┌──────┴──────────────────────────┐
       │ local: modernc SQLite           │
       │ hosted: Turso synced replica    │
       └──────────────┬──────────────────┘
                      │ push / pull
                      ▼
                  Turso Cloud

Vite + Solid 2 + Solid Router + StyleX
              │
              ▼
      internal/web/dist
              │
           go:embed
              │
              ▼
       single Go binary
```

Production is built in two phases: Vite writes the SPA into `internal/web/dist`; then Go embeds that output. The deployable unit is one binary/container.

## Backend boundaries

`cmd/vutame` owns process startup, environment configuration, graceful shutdown, and dependency wiring.

`internal/database` owns SQL backend selection, local SQLite setup, Turso sync lifecycle, and the hosted no-memory-fallback rule.

`internal/httpapi` owns HTTP routing and transport concerns. It depends on interfaces rather than global profile data.

`internal/profile` owns public profile domain rules: stable IDs, handle normalization/validation, link ordering, visibility, ownership-safe editing, and the profile store/editor interfaces.

`internal/auth` owns one-time email challenges, user/session persistence, HMAC/token rules, and the pluggable email sender boundary.

`internal/web` owns the embedded SPA and client-side route fallback.

Future packages should follow the same boundary style: `analytics`, `social`, `billing`, and `atproto` should expose domain services rather than leaking database or protocol details into HTTP handlers.

## API foundation

Public reads:

- `GET /api/v1/healthz`
- `GET /api/v1/meta`
- `GET /api/v1/discover`
- `GET /api/v1/profiles/{handle}`
- `GET /api/v1/handles/{handle}/availability`

Authentication:

- `POST /api/v1/auth/code`
- `POST /api/v1/auth/verify`
- `GET /api/v1/auth/session`
- `POST /api/v1/auth/logout`

Authenticated editing:

- `GET /api/v1/me/profile`
- `POST /api/v1/me/profile/claim`
- `PATCH /api/v1/me/profile`
- `POST /api/v1/me/links`
- `PATCH /api/v1/me/links/{id}`
- `DELETE /api/v1/me/links/{id}`
- `PUT /api/v1/me/links/order`

## Identity and handles

Handles are human-readable routing aliases, not primary keys. Every user/profile receives a stable opaque internal ID. Handles may change without breaking ownership relationships, analytics, or ATProto mappings.

Handle rules are deliberately conservative:

- normalized to lowercase and stripped of a leading `@`;
- 3–32 ASCII characters;
- letters, numbers, `.`, `_`, and `-` only;
- must start and end with a letter or number;
- product/system routes such as `api`, `admin`, `create`, and `discover` are reserved.

Handle claiming is transactional. Database uniqueness is the final authority even if two clients observe the handle as available at the same time.

## Persistence

The development fallback is an injected in-memory seed store. It exists only so the marketing/public-profile shell can run without setup; authenticated editing is disabled in that mode.

Persisted local development uses `modernc.org/sqlite` through `database/sql`.

Hosted production uses `turso.tech/database/tursogo` in sync mode. The application opens a local Turso replica through the standard `database/sql` interface, bootstraps it from Turso Cloud, periodically pushes/pulls changes, and performs a final push during graceful shutdown. This keeps the domain stores database-agnostic while retaining a no-CGO Go build.

A container runs with `VUTAME_ENV=production`, which makes persistent database configuration mandatory. Production can never silently fall back to seeded memory data.

Schema management is external to application startup. Atlas owns the desired schema in `internal/dbschema/schema.sql`; the application only verifies that required tables exist. See `docs/deployment.md` for the production apply sequence.

Current core tables:

```text
users
profiles
links
auth_challenges
sessions
```

Later milestones add social and analytics tables such as:

```text
follows
profile_views
link_clicks
```

## AT Protocol strategy

Vutame is **ATProto-native, not ATProto-required**.

The centralized Vutame account remains the zero-friction onboarding path. ATProto is added as a portability and network layer rather than a signup prerequisite.

Planned sequence:

1. Ship complete centralized profiles and links.
2. Add ATProto OAuth identity linking.
3. Publish Vutame Lexicons, initially `com.vutame.profile` and `com.vutame.link`.
4. Allow opt-in writes of public profile/link records to the user's PDS.
5. Run an AppView/indexer for Vutame records and social discovery.
6. Map Vutame stable user IDs to DIDs without making mutable handles authoritative identity.

## Security model

Public profile reads are anonymous. All mutations require an authenticated session and ownership checks. The browser never submits a user ID as mutation authority; ownership comes from the server-side session and is rechecked at the persistence layer.

Verification codes are HMAC-hashed before storage. Raw session bearer tokens are never stored in the database. Hosted cookies are `HttpOnly`, `Secure`, and `SameSite=Lax`.

URLs and user content are treated as untrusted input. Redirect targets, embeds, uploaded media, and custom themes must be validated/sanitized before render.

Authentication secrets, SMTP credentials, database tokens, billing credentials, and ATProto OAuth secrets remain server-side environment/secret-manager values and are never embedded in the SPA.

## Observability

The Go service uses structured `slog` logging. Database sync failures are warnings rather than code/token dumps; authentication codes are never logged in hosted mode.

Before public beta, add request IDs, latency/status metrics, panic recovery, health/readiness separation, and error reporting. Analytics events for profile views and link clicks belong in the product analytics pipeline rather than application logs.
