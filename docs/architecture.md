# Vutame architecture

## Product split

Vutame intentionally uses two domains for two jobs:

- `vutame.com` — marketing, discovery, onboarding, account management, and the social network.
- `vuta.me/@handle` — the short public profile URL users share everywhere.

Both domains can initially point at the same Go service and embedded Solid SPA. Host-aware canonical redirects can be added once production DNS is wired.

## Runtime

```text
Vite + Solid 2 + Solid Router + StyleX
              │
              ▼
      internal/web/dist
              │
           go:embed
              │
              ▼
       single Go binary
        /api/v1/*
        SPA fallback
```

The first API slice is deliberately small:

- `GET /api/v1/healthz`
- `GET /api/v1/discover`
- `GET /api/v1/profiles/{handle}`

The current profile store is an in-memory seed so the full marketing → discovery → `@handle` route can be exercised before choosing persistence and auth.

## AT Protocol direction

Vutame should be **ATProto-native, not ATProto-required**.

Requiring an existing Bluesky/ATProto identity at signup would add friction that Linktree does not have. Instead:

1. Ship the normal profile product first: accounts, links, themes, analytics, discovery.
2. Add ATProto OAuth as an identity/linking option.
3. Publish Vutame-owned Lexicons under the `com.vutame.*` namespace, for example:
   - `com.vutame.profile`
   - `com.vutame.link`
4. Let users opt in to writing their public Vutame profile/link records to their PDS.
5. Run an AppView/indexer that ingests those records for search, discovery, recommendations, and social features.
6. Keep Vutame's own account system available for people who do not care about ATProto.

That gives Vutame a strong long-term differentiator: a link-in-bio profile whose public identity and data can become portable instead of being trapped in one SaaS database.

## Near-term data model

The centralized MVP should still use stable internal IDs so it can map cleanly to ATProto later:

```text
users
  id
  handle
  display_name
  bio
  avatar_url
  atproto_did nullable

links
  id
  user_id
  label
  url
  kind
  position
  is_active

follows
  follower_user_id
  followed_user_id

profile_views
link_clicks
```

Do not use a mutable handle as the primary key. In ATProto, the durable identity is the DID; Vutame should follow the same principle internally.
