# M4 — Analytics, growth, and creator tools

Status: **in progress**

Goal: give creators measurable value without turning Vutame into a raw surveillance log.

## Slice 1 — privacy-conscious analytics ingestion

- [ ] Add durable profile-view and link-click events.
- [ ] Track public profile views at the server-rendered profile boundary.
- [ ] Route public link clicks through a safe server redirect so JavaScript and no-script profiles behave consistently.
- [ ] Never persist raw IP addresses, full referrer URLs, or raw user-agent strings.
- [ ] Persist only coarse device class and referrer host needed for aggregate reporting.
- [ ] Filter obvious bots/crawlers before analytics ingestion.
- [ ] Ensure disabled, scheduled, or expired links cannot be used through the analytics redirect.
- [ ] Add SQLite and Turso coverage for ingestion and redirects.
- [ ] Exact-head CI and Docker green; merge into `dev`.

## Slice 2 — creator analytics dashboard

- [ ] Add privacy-conscious daily unique-visitor approximation with rotating keyed hashes.
- [ ] Add daily profile-view and link-click time series.
- [ ] Add top-link reporting.
- [ ] Add referrer-host and coarse-device breakdowns.
- [ ] Add creator-owned analytics API endpoints.
- [ ] Add a responsive analytics dashboard with date-range controls.
- [ ] Exclude known bots from creator metrics by default.

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

## M4 exit criteria

Creators can understand profile traffic and link performance, create useful campaign/contact growth tools, use custom domains where supported, export their data, and integrate through appropriately scoped automation surfaces without Vutame storing unnecessary raw visitor data.
