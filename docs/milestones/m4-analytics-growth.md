# M4 — Analytics, growth, and creator tools

Status: **complete**

Goal: give creators measurable value without turning Vutame into a raw surveillance log.

## Slice 1 — privacy-conscious analytics ingestion

- [x] Add durable profile-view and link-click events.
- [x] Track public profile views at the server-rendered profile boundary.
- [x] Route public link clicks through a safe server redirect so JavaScript and no-script profiles behave consistently.
- [x] Never persist raw IP addresses, full referrer URLs, or raw user-agent strings.
- [x] Persist only coarse device class and referrer host needed for aggregate reporting.
- [x] Filter obvious bots/crawlers before analytics ingestion.
- [x] Ensure disabled, scheduled, or expired links cannot be used through the analytics redirect.
- [x] Add SQLite, HTTP, and Turso coverage for ingestion and redirects.
- [x] Exact-head CI and Docker green; merged into `dev`.

## Slice 2 — creator analytics dashboard

- [x] Add privacy-conscious daily unique-visitor approximation with creator/day-scoped rotating keyed hashes.
- [x] Add daily profile-view and link-click time series, top-link reporting, referrer-host, device, and campaign breakdowns.
- [x] Add creator-owned aggregate analytics APIs and responsive 7/30/90-day dashboard.
- [x] Keep visitor hashes private and exclude known bots from metrics.
- [x] Add SQLite, authenticated HTTP, and Turso-engine coverage.
- [x] Exact-head CI and Docker green; merged into `dev`.

## Slice 3 — campaigns and growth blocks

- [x] Add creator UTM/campaign URL helper and bounded campaign attribution.
- [x] Add opt-in contact capture with affirmative consent and exact consent snapshots.
- [x] Keep collected email addresses outside analytics rows.
- [x] Add honeypot/rate-limit protections and prevent suspended creators from collecting contacts.
- [x] Add creator-owned contact lists and finite 30/90/365-day analytics/contact retention with immediate purge.
- [x] Add SQLite, authenticated HTTP, and Turso coverage.
- [x] Exact-head CI and Docker green; merged into `dev`.

## Slice 4 — creator operations

- [x] Add self-service custom domains with DNS TXT ownership proof.
- [x] Serve verified domains directly at `/` and prefer the verified domain for canonical/OG URLs.
- [x] Define verification semantics: the verified badge means control of a supported public identity proof, beginning with DNS-proven custom domains.
- [x] Persist verification history instead of silently mutating a badge with no audit trail.
- [x] Add creator JSON export for account/profile/link/metadata/analytics/contact/domain/integration data while excluding visitor and token hashes.
- [x] Add one-time plaintext API tokens stored only as HMAC hashes.
- [x] Add least-privilege scopes (`profile:read`, `profile:write`, `analytics:read`, `contacts:read`) and bearer read APIs.
- [x] Add HTTPS-only signed webhooks for profile updates and featured links.
- [x] Derive webhook signing secrets from the server secret rather than persisting raw signing keys.
- [x] Add durable webhook delivery rows, public-network-only delivery, HMAC signatures, bounded redirects, retry/backoff, and terminal failure state.
- [x] Add `/operations` creator workspace for domains, exports, tokens, webhooks, and verification history.
- [x] Add SQLite, authenticated HTTP, custom-host rendering, and Turso-engine coverage.
- [x] Exact-head CI and Docker green; ready to merge into `dev`.

## Analytics and growth privacy invariants

- Raw visitor IP addresses are request-time inputs only and are never stored in analytics event rows.
- Raw user-agent strings are not stored; only a coarse device class may be persisted.
- Referrers are reduced to a normalized host; paths, queries, and fragments are discarded.
- Click redirects resolve the canonical link target from Vutame storage rather than accepting an arbitrary destination from the request.
- Stored visitor hashes are creator-scoped/day-scoped and are never returned by creator analytics APIs or exports.
- Contact email addresses are stored only in the contact domain and never copied into analytics events.
- Every stored contact keeps the consent statement shown at submission time.
- API token plaintext is returned once; only an HMAC hash and short display prefix persist.
- Webhook endpoints must be HTTPS and deliveries resolve only public network addresses to limit SSRF exposure.

## M4 exit criteria

**Satisfied.** Creators can understand profile traffic and link performance, create campaign/contact growth tools, use verified custom domains, export their data, and integrate through appropriately scoped automation surfaces without Vutame storing unnecessary raw visitor data.
