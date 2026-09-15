# Vutame documentation

This directory is the source of truth for product and engineering decisions.

- [Architecture](architecture.md) — system boundaries, domains, data ownership, deployment, and AT Protocol direction.
- [Roadmap](roadmap.md) — the full milestone plan from M0 through federation and monetization.
- [Branching and releases](branching-and-releases.md) — `feature/* → dev → main`, release PRs, and version tags.
- [Deployment](deployment.md) — Turso sync, Atlas schema management, hosted environment variables, and rollout order.
- [Billing and entitlements](billing.md) — Stripe setup, entitlement boundaries, webhook security, data ownership, and deployment smoke tests.
- [AT Protocol portability](atproto-portability.md) — published Lexicons, identity semantics, PDS publication, conflict policy, AppView ingestion, unlinking, and migration behavior.
- [M0 — Foundation](milestones/m0-foundation.md) — completed runtime and domain foundation.
- [M1 — Accounts and persistence](milestones/m1-accounts-persistence.md) — completed durable storage, authentication, handle claiming, link CRUD, and production persistence.
- [M2 — Profile design](milestones/m2-profile-design.md) — completed themes, media, rich/featured/scheduled links, sharing, SEO, import, and accessibility/performance work.
- [M3 — Discovery and social graph](milestones/m3-discovery-social-graph.md) — completed follows, discovery/search, activity feeds, cursor pagination, and safety/privacy rollout controls.
- [M4 — Analytics, growth, and creator tools](milestones/m4-analytics-growth.md) — completed privacy-conscious analytics, growth blocks, retention, custom domains, export, scoped tokens, and signed webhooks.
- [M5 — AT Protocol identity and portability](milestones/m5-atproto.md) — completed OAuth identity linking, PDS publication, Jetstream/AppView indexing, portable rendering, and interoperability/migration documentation.
- [M6 — Monetization and production scale](milestones/m6-production-scale.md) — current milestone covering billing, moderation operations, reliability/recovery, and v1 release operations.

**Current roadmap milestone: M6 — Monetization and production scale.**

When implementation and documentation disagree, update the documentation in the same pull request that changes the architecture.
