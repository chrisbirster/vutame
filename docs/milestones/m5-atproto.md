# M5 — AT Protocol identity and portability

Status: **in progress**

Goal: make Vutame identities portable and interoperable with the AT Protocol while keeping the normal Vutame signup and editing experience independent of federation.

Vutame is **ATProto-native, not ATProto-required**. A linked DID is an additional canonical network identity, not a replacement for the Vutame account or stable internal user ID.

## Slice 1 — identity and OAuth linking

- [x] Add AT Protocol persistence for OAuth state, linked accounts, synced records, indexed records, and Jetstream cursors.
- [x] Define `com.vutame.profile` and `com.vutame.link` Lexicons.
- [x] Resolve `did:plc` and hostname-level `did:web` identities.
- [x] Resolve handles through DNS TXT / well-known endpoints.
- [x] Require bidirectional handle verification before treating a handle as valid.
- [x] Discover PDS resource metadata and authorization-server metadata.
- [x] Implement OAuth Authorization Code + PKCE + PAR + DPoP state flow.
- [x] Encrypt OAuth access tokens, refresh tokens, PKCE verifier, and DPoP private keys at rest.
- [ ] Expose OAuth client metadata, resolution, linking, callback, settings, and unlink HTTP APIs.
- [ ] Wire AT Protocol configuration into the Go runtime.
- [ ] Add a creator-facing AT Protocol settings workspace.
- [ ] Cover identity/OAuth invariants on SQLite and the Turso engine.
- [ ] Exact-head CI and Docker build green; merge Slice 1 into `dev`.

## Slice 2 — PDS publication and conflict rules

- [ ] Refresh DPoP-bound access tokens safely.
- [ ] Publish the creator profile as `com.vutame.profile/self` when publication is enabled.
- [ ] Publish active public links as `com.vutame.link` records using stable record keys.
- [ ] Delete or tombstone remote link records when publication requires it.
- [ ] Persist CID/payload/sync metadata for each published record.
- [ ] Enforce `vutame_wins` and `pds_wins` conflict policies explicitly.
- [ ] Add manual sync/status APIs and creator UI.
- [ ] Add interoperability tests against AT-compatible XRPC behavior.

## Slice 3 — ingestion and AppView

- [ ] Consume Jetstream/firehose events for Vutame Lexicons.
- [ ] Persist a durable ingestion cursor.
- [ ] Validate and index portable profile/link records.
- [ ] Build an AppView read model for portable Vutame identities.
- [ ] Resolve/display linked AT handles and DIDs in public Vutame surfaces.
- [ ] Include indexed AT identities in appropriate discovery/search surfaces without duplicating linked local creators.
- [ ] Handle record deletion, account migration, handle changes, malformed records, and replay safely.

## Slice 4 — portability closeout

- [ ] Publish Lexicon source and interoperability documentation.
- [ ] Document what lives only in Vutame versus what can live in a user's PDS.
- [ ] Document unlinking, token revocation expectations, conflict rules, and export/migration behavior.
- [ ] Add end-to-end portability tests covering authorize → publish → ingest → render.
- [ ] Close M5 and advance the roadmap to M6.

## Identity invariants

- The Vutame stable internal user ID remains the ownership key for centralized data.
- The DID is the canonical AT Protocol identity. Handles are mutable display identifiers.
- A handle is shown as verified only when handle → DID and DID document → handle agree.
- AT Protocol OAuth never accepts private-network metadata or arbitrary redirects in hosted mode.
- OAuth state is single-use, short-lived, bound to the initiating Vutame account, and stored only as a hash.
- Raw access tokens, refresh tokens, PKCE verifiers, and DPoP private keys are never stored unencrypted.
- Linking AT Protocol identity remains optional; ordinary Vutame profiles continue to work without it.

## M5 exit criteria

An AT Protocol user can authorize Vutame, opt in to publishing Vutame profile/link records to their PDS, and have Vutame ingest, discover, and render those portable records while preserving clear conflict and identity semantics.
