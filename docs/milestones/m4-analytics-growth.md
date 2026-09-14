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
- [x] Exact-head CI and Docker green; merged into `dev`.

Daily visitor approximation uses `HMAC(secret, creator ID + UTC day + client IP)` and stores only a truncated encoded digest. On Fly.io, the request path accepts a syntactically valid platform `Fly-Client-IP`; otherwise it falls back to the socket remote address and deliberately ignores generic forwarding headers. Because the token rotates each UTC day, range-level `unique_visitors` is the sum of daily unique counts, not a claim that Vutame recognizes the same person across multiple days.

## Slice 3 — campaigns and growth blocks

- [x] Add a UTM/campaign-link helper in the creator Growth workspace.
- [x] Persist only bounded `utm_campaign` labels for attribution; do not persist `utm_source` or `utm_medium` as visitor fields.
- [x] Add creator-configurable public contact/email capture blocks.
- [x] Require affirmative consent for contact submissions and snapshot the exact consent text with each stored address.
- [x] Keep collected email addresses out of analytics event rows.
- [x] Prevent suspended creators from collecting contacts.
- [x] Add honeypot handling and a conservative public submission rate limit.
- [x] Add campaign attribution to aggregate profile-view analytics.
- [x] Add creator-owned recent contact lists without cross-account leakage.
- [x] Add 30/90/365-day analytics and contact retention controls with immediate purge when shortened.
- [x] Purge expired analytics opportunistically as new analytics arrives and expired contacts as contacts are read/created.
- [x] Add SQLite, authenticated HTTP, and Turso-engine coverage for consent, retention, contact isolation, and campaign reporting.
- [ ] Exact-head CI and Docker green; merge into `dev`.

The creator Growth workspace lives at `/growth`. Public contact capture is opt-in and appears only when a creator explicitly enables it. The public request includes a hidden honeypot and consent checkbox; automated honeypot submissions receive a generic success response without storage so Vutame does not disclose its spam decision.

Retention settings are finite rather than “forever”: 30, 90, or 365 days. Shortening a retention period immediately removes older creator-owned rows. The schema additions (`campaign`, `creator_data_settings`, `contact_blocks`, and `contact_submissions`) require an Atlas schema apply before deploying Slice 3.

## Slice 4 — creator operations

- [ ] Add custom-domain and canonical-domain handling.
- [ ] Define creator verification policy/workflow.
- [ ] Add export of profile and analytics data.
- [ ] Add scoped API tokens and webhooks for advanced integrations.

## Analytics and growth privacy invariants

- Raw visitor IP addresses are request-time inputs only and are never stored in analytics event rows.
- Raw user-agent strings are not stored; only a coarse device class may be persisted.
- Referrers are reduced to a normalized host; paths, queries, and fragments are discarded.
- Click redirects resolve the canonical link target from Vutame storage rather than accepting an arbitrary destination from the request.
- Bot filtering is conservative and metrics remain approximate rather than pretending to identify a person.
- Analytics write failures never prevent a public profile from rendering or a valid public link from redirecting.
- Stored visitor hashes are creator-scoped and day-scoped and are never returned by creator analytics APIs.
- Contact email addresses are stored only in the contact domain and never copied into analytics events.
- Every stored contact keeps the consent statement that was shown when the visitor submitted it.
- Campaign attribution stores a bounded campaign label, not arbitrary profile query strings or a browsing history.

## M4 exit criteria

Creators can understand profile traffic and link performance, create useful campaign/contact growth tools, use custom domains where supported, export their data, and integrate through appropriately scoped automation surfaces without Vutame storing unnecessary raw visitor data.
