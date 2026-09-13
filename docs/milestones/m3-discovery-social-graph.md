# M3 — Discovery and social graph

Status: **in progress**

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
- [ ] Discovery-pagination exact-head CI green and merge into `dev`.

## Slice 3 — safety and rollout controls

- [ ] Add block and mute relationships before broad social rollout.
- [ ] Make blocked users invisible to each other in follow/discovery/feed paths.
- [ ] Add report primitives and moderation/admin state.
- [ ] Add rate limits and anti-spam protections for follow/search mutations.
- [ ] Add privacy controls needed for broader discovery.

## M3 exit criteria

A signed-in user can find creators, follow/unfollow them, browse follower/following relationships, and consume a useful discovery/following experience. Safety primitives must exist before the social graph is treated as broadly public infrastructure.
