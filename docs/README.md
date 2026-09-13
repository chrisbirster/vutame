# Vutame documentation

This directory is the source of truth for product and engineering decisions.

- [Architecture](architecture.md) — system boundaries, domains, data ownership, deployment, and AT Protocol direction.
- [Roadmap](roadmap.md) — the full milestone plan from M0 through federation and monetization.
- [Branching and releases](branching-and-releases.md) — `feature/* → dev → main`, release PRs, and version tags.
- [Deployment](deployment.md) — Turso sync, Atlas schema management, hosted environment variables, and rollout order.
- [M0 — Foundation](milestones/m0-foundation.md) — completed runtime and domain foundation.
- [M1 — Accounts and persistence](milestones/m1-accounts-persistence.md) — completed durable storage, authentication, handle claiming, link CRUD, and production persistence.
- [M2 — Profile design](milestones/m2-profile-design.md) — current milestone for themes, media, richer links, sharing, SEO, and accessibility.

When implementation and documentation disagree, update the documentation in the same pull request that changes the architecture.
