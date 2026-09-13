# M3 — Discovery and social graph

Status: **complete**

Goal: turn Vutame from isolated creator profiles into a browsable creator network with explicit follows, searchable identity metadata, useful activity feeds, and trustworthy discovery primitives.

## Slice 1 — follow graph and searchable discovery

- [x] Add durable follow relationships with uniqueness and self-follow protection.
- [x] Add follow/unfollow APIs derived from the authenticated session.
- [x] Add follower/following counts to public creator summaries.
- [x] Add follower/following list endpoints.
- [x] Add profile categories/interests as searchable creator metadata.
- [x] Add public creator search by handle, display name, bio, category, and interest.
- [x] Add a real discovery/search UI with follow controls for signed-in users.
- [x] Add SQLite and Turso coverage for follow/search behavior.
- [x] Add authenticated HTTP lifecycle coverage.
- [x] CI green on the exact feature head and merge into `dev`.

## Slice 2 — activity and feeds

- [x] Persist bounded public activity events for profile changes and newly featured links.
- [x] Record activity through an editor decorator so successful profile/link mutations are not coupled to feed delivery.
- [x] Add a signed-in following activity feed.
- [x] Add a public recent-activity surface.
- [x] Add a transparent trending heuristic based on seven-day public activity plus follower count.
- [x] Add opaque cursor pagination to following and recent activity feeds.
- [x] Add a responsive `/feed` UI with Following, Recent, Trending, and load-more states.
- [x] Add SQLite and Turso coverage for activity/feed behavior.
- [x] Activity-feed exact-head CI and Docker gate green; merged into `dev`.
- [x] Add cursor pagination to creator search/discovery results with duplicate-safe load-more UI.
- [x] Add SQLite, HTTP, and Turso coverage for discovery cursors.
- [x] Discovery-pagination exact-head CI and Docker gate green; merged into `dev`.

## Slice 3 — safety and rollout controls

- [x] Add durable block and mute relationships before broad social rollout.
- [x] Make blocked users invisible to each other in follow, discovery, social-summary, and feed paths.
- [x] Make muting remove a creator from viewer-specific feeds without changing the follow relationship.
- [x] Remove follows in both directions when a block is created.
- [x] Add creator privacy controls for discovery, public activity, and inbound follows.
- [x] Remove existing inbound follows when a creator disables follows.
- [x] Add bounded report primitives with explicit reasons and persisted report state.
- [x] Add durable moderation profile state (`active`, `restricted`, `suspended`) without exposing an insecure admin mutation surface.
- [x] Add rate limits for public discovery search and authenticated social/report mutations.
- [x] Add a creator-facing `/settings/safety` UI for privacy, block, mute, and report controls.
- [x] Add SQLite coverage for safety persistence and policy semantics.
- [x] Add Turso integration coverage crossing safety, social, and activity behavior.
- [x] Add authenticated HTTP lifecycle and deterministic `429` coverage.
- [x] Exact-head application verification and Docker gate green on the safety feature implementation.
- [x] M3 closeout documentation advanced to M4; final closeout head must remain green before merge into `dev`.

### Safety implementation notes

Blocking is the strongest relationship boundary: it removes existing follows in both directions and prevents future follow/social visibility between the two accounts. Muting is intentionally weaker and affects viewer-specific feeds only. Privacy controls are creator-owned and can independently disable discovery, public activity, or inbound follows.

The current abuse limiter is process-local and appropriate for the current deployment shape. A horizontally scaled deployment should move shared rate-limit state to infrastructure designed for distributed counters.

Moderation state is durable and enforced by social/activity policy, but M3 deliberately does not expose an ad-hoc admin HTTP endpoint. A future moderation console must use a real staff/admin authorization model.

## M3 exit criteria

**Satisfied.** A signed-in user can find creators, follow/unfollow them, browse follower/following relationships, consume discovery/following feeds, block or mute creators, control discovery/activity/follow privacy, and report abuse. Block/privacy/moderation policy is enforced in the read paths rather than existing only as UI state. M4 is the active roadmap milestone.
