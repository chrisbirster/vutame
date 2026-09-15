# M5 — AT Protocol identity and portability

Status: **complete**

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
- [x] Expose OAuth client metadata, resolution, linking, callback, settings, and unlink HTTP APIs.
- [x] Wire AT Protocol configuration into the Go runtime.
- [x] Add a creator-facing AT Protocol settings workspace.
- [x] Cover identity/OAuth invariants on SQLite and the Turso engine.
- [x] Exact-head CI and Docker build green; merged into `dev` as PR #26.

## Slice 2 — PDS publication and conflict rules

- [x] Refresh DPoP-bound access tokens safely with serialized single-use refresh-token rotation.
- [x] Publish the creator profile as `com.vutame.profile/self` when publication is enabled.
- [x] Publish currently active/public links as `com.vutame.link` records using stable record keys.
- [x] Delete Vutame-managed remote link records when the local link is no longer public under the active conflict policy.
- [x] Persist CID, canonical payload, local update time, and sync metadata for each managed record.
- [x] Enforce `vutame_wins` and `pds_wins` explicitly; `pds_wins` preserves remote divergence and reports it rather than overwriting it.
- [x] Add manual sync/status APIs and creator UI with managed-record CIDs and conflict reporting.
- [x] Add protocol-level interoperability tests against AT-compatible DPoP/XRPC behavior.
- [x] Exact-head CI and Docker green; merged into `dev` as PR #28.

Publication remains opt-in. Enabling publication does not silently write records; creators use **Sync now** to make the current Vutame public profile state portable. Vutame-managed link record keys are deterministic hashes of stable internal link IDs, so label/URL edits do not create duplicate PDS records.

Conflict semantics are deliberately asymmetric and observable. `vutame_wins` treats the centralized editor as authoritative and rewrites managed PDS records. `pds_wins` detects a CID change/deletion since the previous Vutame sync, preserves the remote outcome, records the new remote baseline, and reports the conflict to the creator. A later local edit can be synchronized against that new baseline.

## Slice 3 — ingestion and AppView

- [x] Consume Jetstream events for Vutame Lexicons with a hardened WebSocket client.
- [x] Persist a monotonic durable ingestion cursor and reconnect from that cursor.
- [x] Share strict portable profile/link record validation and AppView storage with outbound sync.
- [x] Build the AppView read model for portable Vutame identities.
- [x] Resolve/display linked AT DIDs and current verified handles in public Vutame surfaces.
- [x] Include indexed portable identities in discovery/search without duplicating locally linked DIDs.
- [x] Handle record deletion, account deactivation/takedown, mutable handle refresh, malformed records, and replay safely.
- [x] Render portable profiles at `/at/:did` with the shared Vutame profile surface while sending portable links directly to their record URL.
- [x] Cover ingestion/AppView behavior on SQLite and the Turso engine.

## Slice 4 — portability closeout

- [x] Publish Lexicon source and interoperability documentation.
- [x] Document what lives only in Vutame versus what can live in a user's PDS.
- [x] Document unlinking, token revocation expectations, conflict rules, and export/migration behavior.
- [x] Add end-to-end portability coverage for authorize → publish → ingest → AppView render model.
- [x] Exact-head application CI and Docker green for the final M5 implementation.
- [x] Close M5 and advance the roadmap to M6.

See [`../atproto-portability.md`](../atproto-portability.md) for the interoperability and migration contract.

## Identity invariants

- The Vutame stable internal user ID remains the ownership key for centralized data.
- The DID is the canonical AT Protocol identity. Handles are mutable display identifiers.
- A handle is shown as verified only when handle → DID and DID document → handle agree.
- AT Protocol OAuth never accepts private-network metadata or arbitrary redirects in hosted mode.
- OAuth state is single-use, short-lived, bound to the initiating Vutame account, and stored only as a hash.
- Raw access tokens, refresh tokens, PKCE verifiers, and DPoP private keys are never stored unencrypted.
- OAuth refresh tokens are treated as single-use and refreshed under a process mutex to prevent concurrent reuse.
- PDS requests use DPoP-bound access tokens and require server-provided DPoP nonces.
- Jetstream ingestion is at-least-once/replay-safe and indexes only validated Vutame Lexicon records.
- Portable records cannot self-assert Vutame verification.
- Linking AT Protocol identity remains optional; ordinary Vutame profiles continue to work without it.

## M5 exit criteria

**Satisfied.** An AT Protocol user can authorize Vutame, opt in to publishing Vutame profile/link records to their PDS, and have Vutame ingest, discover, and render those portable records while preserving clear conflict and identity semantics.
