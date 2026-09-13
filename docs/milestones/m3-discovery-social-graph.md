# M3 — Discovery and social graph

Status: **in progress**

Goal: turn Vutame from isolated creator profiles into a browsable creator network with explicit follows, searchable identity metadata, and trustworthy discovery primitives.

## Slice 1 — follow graph and searchable discovery

- [ ] Add durable follow relationships with uniqueness and self-follow protection.
- [ ] Add follow/unfollow APIs derived from the authenticated session.
- [ ] Add follower/following counts to public creator summaries.
- [ ] Add follower/following list endpoints.
- [ ] Add profile categories/interests as searchable creator metadata.
- [ ] Add public creator search by handle, display name, bio, category, and interest.
- [ ] Add a real discovery/search UI with follow controls for signed-in users.
- [ ] Add SQLite and Turso coverage for follow/search behavior.
- [ ] Add authenticated HTTP lifecycle coverage.
- [ ] CI green on the exact feature head and merge into `dev`.

## Slice 2 — activity and feeds

- [ ] Persist public activity events for profile changes and newly featured links.
- [ ] Add following activity feed.
- [ ] Add recent creator/activity discovery surfaces.
- [ ] Add trending heuristics that do not require M4 analytics infrastructure.
- [ ] Add pagination/cursors for discovery and feed endpoints.

## Slice 3 — safety and rollout controls

- [ ] Add block and mute relationships before broad social rollout.
- [ ] Make blocked users invisible to each other in follow/discovery/feed paths.
- [ ] Add report primitives and moderation/admin state.
- [ ] Add rate limits and anti-spam protections for follow/search mutations.
- [ ] Add privacy controls needed for broader discovery.

## M3 exit criteria

A signed-in user can find creators, follow/unfollow them, browse follower/following relationships, and consume a useful discovery/following experience. Safety primitives must exist before the social graph is treated as broadly public infrastructure.
