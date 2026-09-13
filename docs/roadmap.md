# Vutame product and engineering roadmap

The roadmap is ordered so Vutame first reaches Linktree-level utility, then adds the social/discovery and portability features that make it distinct.

## M0 — Foundation

Goal: establish a boring, testable platform that every later feature can build on.

Tasks:

- Go 1.26 `net/http` service with graceful shutdown.
- Vite-built Solid 2 SPA embedded with `go:embed`.
- Solid Router, StyleX, TypeScript, SPA deep-link fallback.
- `vutame.com` / `vuta.me` product split documented and configurable.
- `profile.Store` abstraction with an in-memory M0 implementation.
- Stable opaque profile/link IDs; handles remain mutable aliases.
- Handle normalization, validation, reserved names, and availability API.
- Public profile/discovery read APIs plus health/meta endpoints.
- Seed profile proving `/@handle` end to end.
- CI for typecheck, Go tests, Vite production build, and Go build.
- `feature/* → dev → main` release discipline documented.

Exit criteria: the app builds as one binary, CI is green, profile deep links work, the API is covered by tests, and future persistence can replace the memory store without rewriting HTTP handlers.

## M1 — Accounts, persistence, and real link CRUD

Goal: let a real person claim and manage a Vuta.

Tasks:

- SQLite local persistence and libSQL/Turso production adapter.
- Atlas desired schema and explicit schema apply workflow.
- `users`, `profiles`, `links`, and `sessions` schema.
- Email magic-link/code authentication.
- Secure session cookies, logout, session expiry, CSRF strategy for mutations.
- Claim-handle transaction with uniqueness enforcement.
- Create/read/update/delete links.
- Reorder links and toggle visibility.
- Edit display name, bio, avatar, and profile metadata.
- Account/profile settings screen.
- Ownership/authorization tests and database integration tests.

Exit criteria: a new user can sign in, claim `vuta.me/@name`, add/reorder links, sign out, and see the same persisted public profile after restart.

## M2 — Linktree parity and profile design

Status: **complete**

Goal: make Vutame useful enough to replace a conventional link-in-bio product.

Tasks:

- Theme system with tokens for background, typography, buttons, radius, and spacing.
- Profile editor with live mobile preview.
- Avatar/media upload pipeline.
- Social icon links and typed link kinds.
- Link thumbnails and rich preview cards.
- Scheduled links and temporary visibility windows.
- Featured/pinned links.
- Provider-aware GitHub/YouTube/Spotify/TikTok and generic OpenGraph enrichment where safe, rendered as Vutame-native cards rather than arbitrary third-party iframe code.
- QR code generation.
- SEO/OG metadata for public profiles.
- Accessibility and mobile performance pass.
- Importer for Linktree-style exported/link lists where legally/technically possible.

Exit criteria: **satisfied.** A creator can reproduce a polished existing link-in-bio page without losing core presentation features.

## M3 — Discovery and social graph

Status: **next**

Goal: turn isolated link pages into a network.

Tasks:

- Follow/unfollow users.
- Follower/following pages.
- Creator categories, tags, and searchable interests.
- Search by handle, display name, bio, and linked projects.
- Discover/trending/recent creator surfaces.
- Activity events for profile changes and newly featured links.
- Optional following feed.
- Blocks/mutes and privacy controls before broad social rollout.
- Moderation/admin primitives and report flow.
- Anti-spam/rate-limit protections.

Exit criteria: users have a reason to browse `vutame.com` even when they did not arrive through somebody's `vuta.me` link.

## M4 — Analytics, growth, and creator tools

Goal: give creators measurable value and growth tooling.

Tasks:

- Profile view and link click event ingestion.
- Bot filtering and privacy-conscious unique visitor approximation.
- Time-series analytics dashboard.
- Referrer, device, country/region, and top-link reporting where permitted.
- UTM helper and campaign links.
- Email/contact capture blocks with consent controls.
- Custom domains and canonical-domain handling.
- Creator verification policy/workflow.
- Export of profile and analytics data.
- Webhooks/API tokens for advanced integrations.

Exit criteria: creators can understand what is working and use Vutame as an operational creator tool, not just a static page.

## M5 — AT Protocol identity and portability

Goal: make Vutame profiles portable and interoperable without forcing federation on ordinary users.

Tasks:

- ATProto OAuth login/linking flow.
- DID mapping on Vutame accounts.
- Define and publish `com.vutame.profile` Lexicon.
- Define and publish `com.vutame.link` Lexicon.
- Opt-in publish/sync of public records to user PDS repositories.
- Conflict/version rules between centralized editor state and AT records.
- Jetstream/firehose ingestion for Vutame Lexicons.
- Vutame AppView/indexer for search and discovery.
- Resolve AT handles/DIDs and display linked identities.
- Portability/export documentation and interoperability tests.

Exit criteria: an ATProto user can authorize Vutame, publish their profile data to their PDS, and have Vutame discover/render those portable records.

## M6 — Monetization and production scale

Goal: build a sustainable hosted product around the free identity layer.

Tasks:

- Free/paid plan boundaries.
- Billing provider integration and entitlement service.
- Paid themes/custom branding removal/custom domains/advanced analytics as candidate premium features.
- Team/organization profiles where product demand supports them.
- Abuse operations, moderation queues, and takedown processes.
- Backup/restore and disaster recovery exercises.
- SLOs, dashboards, tracing/error reporting, capacity/load tests.
- CDN/cache strategy for high-volume public profiles.
- Security review, dependency policy, privacy policy, terms, and data retention tooling.
- Production deployment/release automation tied to version tags.

Exit criteria: Vutame can be run as a reliable public service with clear free value, paid upgrades, moderation, observability, and repeatable releases.
