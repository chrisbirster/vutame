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
       ┌──────┴────────┐
       │ profile.Store │  ← interface boundary
       └──────┬────────┘
              │
       memory store (M0)
              │
       SQLite/libSQL (M1)

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

`internal/httpapi` owns HTTP routing and transport concerns. It depends on interfaces rather than global profile data.

`internal/profile` owns public profile domain rules: stable IDs, handle normalization/validation, link ordering, visibility, and the profile store interface.

`internal/web` owns the embedded SPA and client-side route fallback.

Future packages should follow the same boundary style: `account`, `auth`, `analytics`, `social`, `billing`, and `atproto` should expose domain services rather than leaking database or protocol details into HTTP handlers.

## Public API foundation

M0 exposes:

- `GET /api/v1/healthz`
- `GET /api/v1/meta`
- `GET /api/v1/discover`
- `GET /api/v1/profiles/{handle}`
- `GET /api/v1/handles/{handle}/availability`

Mutation APIs arrive with authenticated accounts in M1.

## Identity and handles

Handles are human-readable routing aliases, not primary keys. Every user/profile receives a stable opaque internal ID. Handles may change without breaking ownership relationships, analytics, or ATProto mappings.

M0 handle rules are deliberately conservative:

- normalized to lowercase and stripped of a leading `@`;
- 3–32 ASCII characters;
- letters, numbers, `.`, `_`, and `-` only;
- must start and end with a letter or number;
- product/system routes such as `api`, `admin`, `create`, and `discover` are reserved.

## Persistence plan

M0 uses an injected in-memory store so domain behavior is testable before persistence is chosen.

M1 moves the same interfaces to SQLite locally and libSQL/Turso for hosted production. Schema management should be explicit and external to normal application startup; the preferred direction is an Atlas-owned desired schema, following the pattern used in the other Go projects.

Core tables are expected to include:

```text
users
  id
  email
  created_at
  updated_at

profiles
  user_id
  handle unique
  display_name
  bio
  avatar_url
  verified
  atproto_did nullable

links
  id
  user_id
  label
  url
  kind
  position
  is_active
  created_at
  updated_at

follows
  follower_user_id
  followed_user_id
  created_at

profile_views
link_clicks
sessions
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

Public profile reads are anonymous. All future mutations require an authenticated session and ownership checks. URLs and user content are treated as untrusted input. Redirect targets, embeds, uploaded media, and custom themes must be validated/sanitized before render.

Authentication secrets, email provider credentials, billing credentials, and ATProto OAuth secrets remain server-side environment/secret-manager values and are never embedded in the SPA.

## Observability

The Go service uses structured `slog` logging. Before public beta, add request IDs, latency/status metrics, panic recovery, health/readiness separation, and error reporting. Analytics events for profile views and link clicks belong in the product analytics pipeline rather than application logs.
