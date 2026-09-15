# M6 — Monetization and production scale

Status: **in progress**

Goal: make Vutame sustainable and operable as a reliable public service without weakening the free portable identity layer.

## Slice 1 — plans, billing, and entitlements

- [x] Define durable `free` and `pro` plan boundaries.
- [x] Add an entitlement service independent of the billing provider.
- [x] Integrate Stripe-hosted Checkout for subscription creation.
- [x] Integrate Stripe Customer Portal for subscription management.
- [x] Verify Stripe webhook signatures against the raw request body.
- [x] Process subscription webhooks idempotently and persist a provider event ledger.
- [x] Keep billing provider customer/subscription IDs separate from Vutame user IDs.
- [x] Add a Billing workspace showing current plan, subscription state, and feature entitlements.
- [x] Make custom domains a Pro entitlement when billing is configured.
- [x] Make analytics history beyond 30 days a Pro entitlement when billing is configured.
- [x] Keep core profile/link publishing, social graph, safety, export, and AT Protocol portability free.
- [x] Cover billing/entitlement behavior on SQLite, HTTP, and Turso paths.
- [x] Exact-head CI and Docker green; merge into `dev`.

Slice 1 merged through PR #30 as `370d2d024d48141e24668603f9831e64d9bcf593` after exact-head CI and Docker verification on `528be96c81e29861e854a743785f215b4c4ca954`.

## Slice 2 — abuse and moderation operations

- [x] Add an explicit admin authorization model; never expose moderation mutation APIs through a shared query/header secret.
- [x] Add a moderation queue for open/reviewing reports.
- [x] Add report assignment, notes, resolution/dismissal, restriction/suspension, and takedown audit history.
- [x] Add public-content takedown behavior that consistently affects profiles, discovery, feeds, AppView-linked local identities, contact capture, and custom domains.
- [x] Add abuse-rate dashboards/counters without storing unnecessary raw user data.
- [x] Add creator appeal/contact workflow primitives.
- [x] Cover moderation authorization and policy propagation in SQLite/HTTP/Turso tests.
- [ ] Exact-head CI and Docker green; merge into `dev`.

## Slice 3 — reliability, observability, and recovery

- [ ] Define service-level indicators/objectives for public profile reads, API availability, and mutation latency.
- [ ] Add structured request IDs and server metrics suitable for scraping/aggregation.
- [ ] Add health/readiness separation and dependency status without leaking secrets.
- [ ] Add bounded error logging/tracing hooks with privacy-safe attributes.
- [ ] Add repeatable load tests for public profiles, discovery, auth-sensitive APIs, and hot-link redirects.
- [ ] Document CDN/cache policy for immutable media, public profiles, discovery, and authenticated/private responses.
- [ ] Add backup/restore runbooks covering Turso/libSQL state and managed media.
- [ ] Add automated backup verification metadata and a destructive local restore drill script/test.
- [ ] Document RPO/RTO targets and disaster-recovery exercise procedure.
- [ ] Exact-head CI and Docker green; merge into `dev`.

## Slice 4 — security, policy, and v1 release operations

- [ ] Add dependency/update policy and CI dependency vulnerability auditing.
- [ ] Add production security review/checklist covering auth, SSRF, webhooks, OAuth/DPoP, billing, media, admin access, cookies, and secrets.
- [ ] Publish repository privacy-policy and terms templates that accurately describe current data handling.
- [ ] Consolidate data-retention/deletion/export behavior and user account deletion runbook.
- [ ] Add version metadata to runtime `/api/v1/meta` and build artifacts.
- [ ] Add tag-driven production release workflow with build/test/Docker artifact generation.
- [ ] Add release checklist requiring Atlas plan/apply, backup verification, smoke tests, rollback target, and changelog.
- [ ] Add `v1.0.0` release notes/checklist documentation.
- [ ] Team/organization profiles remain explicitly deferred until product demand justifies their permission/data model.
- [ ] Exact-head CI and Docker green; close M6.

## Free identity invariant

Payment never controls ownership of a Vutame handle, basic public profile/link rendering, data export, safety controls, or AT Protocol portability. Paid features add operational value; they do not hold a creator's identity or portable public records hostage.

## M6 exit criteria

Vutame can be operated as a reliable public service with clear free value, an optional paid upgrade, authenticated moderation operations, tested recovery procedures, observable SLOs, documented security/privacy policy, and a repeatable version-tagged production release process.
