# M4 — Analytics, growth, and creator tools

Status: **in progress**

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

Profile-view events are currently recorded at the origin HTML boundary. The profile response retains its short public cache window, so a future CDN/edge-cache rollout must deliberately decide whether analytics moves to the edge or accepts origin-level undercounting rather than silently treating cached responses as origin views.

## Slice 2 — creator analytics dashboard

- [x] Add privacy-conscious daily unique-visitor approximation with rotating keyed hashes.
- [x] Scope visitor hashes to both creator and UTC day so stored values are not cross-creator or long-lived identifiers.
- [x] Add daily profile-view and link-click time series.
- [x] Add top-link reporting.
- [x] Add referrer-host and coarse-device breakdowns.
- [x] Add creator-owned analytics API endpoints with 1–90 day range bounds.
- [x] Add a responsive analytics dashboard with 7/30/90-day controls.
- [x] Keep the dashboard aggregate-only; do not expose stored visitor hashes or event-level visitor data.
- [x] Exclude known bots from creator metrics by default.
- [x] Add SQLite, authenticated HTTP, and Turso-engine coverage for dashboard behavior.
- [x] Exact-head CI and Docker green; ready to merge into `dev`.

Daily visitor approximation uses `HMAC(secret, creator ID + UTC day + client IP)` and stores only a truncated encoded digest. On Fly.io, the request path accepts a syntactically valid platform `Fly-Client-IP`; otherwise it falls back to the socket remote address and deliberately ignores generic forwarding headers. Because the token rotates each UTC day, range-level `unique_visitors` is the sum of daily unique counts, not a claim that Vutame recognizes the same person across multiple days.

Adding `visitor_hash` changes the desired Atlas schema. Apply the schema before deploying an app build containing Slice 2.

## Slice 3 — campaigns and growth blocks

- [ ] Add UTM/campaign-link helper.
- [ ] Add contact/email capture blocks with explicit consent text.
- [ ] Add campaign attribution to aggregate analytics where technically and legally appropriate.
- [ ] Add creator controls for analytics/contact retention.

## Slice 4 — creator operations

- [ ] Add custom-domain and canonical-domain handling.
- [ ] Define creator verification policy/workflow.
- [ ] Add export of profile and analytics data.
- [ ] Add scoped API tokens and webhooks for advanced integrations.

## Analytics privacy invariants

- Raw visitor IP addresses are request-time inputs only and are never stored in analytics event rows.
- Raw user-agent strings are not stored; only a coarse device class may be persisted.
- Referrers are reduced to a normalized host; paths, queries, and fragments are discarded.
- Click redirects resolve the canonical link target from Vutame storage rather than accepting an arbitrary destination from the request.
- Bot filtering is conservative and metrics remain approximate rather than pretending to identify a person.
- Analytics write failures never prevent a public profile from rendering or a valid public link from redirecting.
- Stored visitor hashes are creator-scoped and day-scoped and are never returned by creator analytics APIs.

## M4 exit criteria

Creators can understand profile traffic and link performance, create useful campaign/contact growth tools, use custom domains where supported, export their data, and integrate through appropriately scoped automation surfaces without Vutame storing unnecessary raw visitor data.
