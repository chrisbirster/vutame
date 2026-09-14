# Vutame product and engineering roadmap

The roadmap is ordered so Vutame first reaches Linktree-level utility, then adds the social/discovery and portability features that make it distinct.

## M0 — Foundation

Status: **complete**

Goal: establish a boring, testable platform that every later feature can build on.

Exit criteria: satisfied.

## M1 — Accounts, persistence, and real link CRUD

Status: **complete**

Goal: let a real person claim and manage a Vuta.

Exit criteria: satisfied.

## M2 — Linktree parity and profile design

Status: **complete**

Goal: make Vutame useful enough to replace a conventional link-in-bio product.

Exit criteria: **satisfied.** A creator can reproduce a polished existing link-in-bio page without losing core presentation features.

## M3 — Discovery and social graph

Status: **complete**

Goal: turn isolated link pages into a network.

Exit criteria: **satisfied.** Users have a reason to browse `vutame.com` even when they did not arrive through somebody's `vuta.me` link, and the resulting network has block/mute/privacy/report/moderation primitives enforced in its read and mutation paths.

## M4 — Analytics, growth, and creator tools

Status: **complete**

Goal: give creators measurable value and growth tooling.

Delivered:

- Privacy-conscious profile-view and link-click analytics.
- Bot filtering, rotating creator/day visitor approximation, time series, top links, referrers, devices, and campaign reporting.
- UTM/campaign URL helper and consented contact capture.
- Finite analytics/contact retention controls.
- DNS-verified custom domains and canonical-domain handling.
- Public-identity verification policy and audit history.
- Creator JSON export.
- HMAC-hashed scoped API tokens and bearer read APIs.
- HTTPS-only signed webhooks with durable retry/backoff.

Exit criteria: **satisfied.** Creators can understand what is working and use Vutame as an operational creator tool, not just a static page.

## M5 — AT Protocol identity and portability

Status: **current**

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

Status: **next**

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
