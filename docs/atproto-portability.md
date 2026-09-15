# AT Protocol portability contract

Vutame is **ATProto-native, not ATProto-required**. A normal Vutame account works without an AT Protocol identity. Linking a DID adds a portable public-data layer; it does not replace Vutame's internal user ID or authentication system.

## Published Lexicons

The source Lexicons live in the repository:

- `lexicons/com/vutame/profile.json`
- `lexicons/com/vutame/link.json`

Vutame publishes one profile record at:

```text
com.vutame.profile/self
```

and zero or more link records in:

```text
com.vutame.link/<stable-rkey>
```

The link record key is deterministically derived from Vutame's stable internal link ID. Editing a label or URL therefore updates the same portable record instead of creating a new identity for that link.

## Identity model

Vutame has two identity layers:

1. The Vutame `user_id` is the ownership key for centralized Vutame data.
2. The DID is the stable AT Protocol identity for portable records.

AT handles are mutable aliases. Vutame displays an AT handle as verified only when handle → DID resolution and the DID document's `alsoKnownAs` handle agree.

A portable record cannot self-assert Vutame verification. The `verified` field is accepted for schema compatibility but the AppView derives trust from Vutame's own identity proof rules.

## OAuth and authorization

Vutame uses AT Protocol OAuth Authorization Code flow with:

- PKCE S256;
- pushed authorization requests (PAR);
- DPoP-bound access tokens;
- server-provided DPoP nonces;
- rotating refresh tokens;
- granular repository scopes for `com.vutame.profile` and `com.vutame.link`.

OAuth state is single-use and bound to the initiating Vutame account. Browser-visible state is stored only as a hash. PKCE verifiers, access tokens, refresh tokens, and DPoP private keys are encrypted at rest.

Refresh tokens are treated as single-use. Refreshes are serialized inside one Vutame process and the persisted credentials are reloaded after acquiring that lock so concurrent syncs do not intentionally reuse the same refresh token.

## What is portable

The portable profile record contains the public presentation fields required to render a Vuta elsewhere:

- display name;
- public handle text;
- bio;
- avatar URL;
- theme identifier;
- update timestamp.

Portable link records contain:

- label;
- destination URL;
- typed link kind;
- optional thumbnail URL;
- featured state;
- ordering position;
- update timestamp.

Only links that are active and currently inside their public visibility window are published.

## What remains Vutame-only

The following are intentionally centralized and are not copied into PDS records:

- Vutame account/session authentication;
- email address and login challenges;
- follower graph, blocks, mutes, reports, and moderation state;
- analytics events and rotating visitor hashes;
- contact-capture submissions and consent records;
- custom-domain verification tokens/history;
- API tokens and webhook configuration/delivery history;
- media-storage internals;
- billing/entitlement data;
- internal database IDs other than the opaque identity needed to derive stable portable link record keys.

A Vutame data export can include centralized creator-owned data separately. AT portability is not a claim that every Vutame product feature is federated.

## Publication model

Publication is opt-in. Enabling `publish_enabled` does not write to the PDS by itself. The creator explicitly uses **Sync now**.

A sync:

1. refreshes the OAuth session when required;
2. publishes `com.vutame.profile/self`;
3. publishes all currently public Vutame-managed link records;
4. removes stale Vutame-managed remote link records when the selected conflict policy permits it;
5. records the remote CID, canonical payload, local update timestamp, and sync timestamp.

The managed-record ledger exists so later syncs can distinguish Vutame's previous write from an external PDS edit.

## Conflict policies

### `vutame_wins`

The centralized Vutame editor is authoritative for Vutame-managed records. A sync rewrites the managed PDS record to match the current public Vutame state.

### `pds_wins`

Before changing an already-managed record, Vutame compares the current remote CID with the CID from its previous successful sync.

If the remote record changed or was deleted outside Vutame, Vutame:

- does not overwrite that external result;
- updates its remote baseline when appropriate;
- reports a conflict to the creator.

A later explicit local edit/sync can proceed against the new baseline. This policy is intentionally observable rather than silently implementing last-write-wins.

## AppView and Jetstream ingestion

When `VUTAME_ATPROTO_JETSTREAM_URL` is configured, Vutame consumes only these collections:

- `com.vutame.profile`
- `com.vutame.link`

The consumer keeps a durable microsecond cursor and reconnects with that cursor. Index writes are idempotent so at-least-once replay is safe.

For commit events:

- create/update validates the portable record before indexing;
- delete removes that indexed record;
- malformed Vutame records are skipped without becoming AppView content.

For account events, deactivated or taken-down DIDs are removed from the AppView.

Identity events cause Vutame to re-resolve the DID when possible so a mutable handle can be refreshed independently of the record body.

The AppView exposes portable profiles by DID and includes portable-only creators in discovery. If a DID is already linked to a local Vutame creator, the portable search result is suppressed to avoid duplicate identities.

## Rendering

Portable profiles use the same Vutame `ProfileSurface` as native public profiles so themes, accessibility, and card rendering stay consistent.

Portable links go directly to their record URL. They do not pass through Vutame's local `/out/{linkID}` analytics redirect because a portable AppView record is not a Vutame-owned local link row.

## Unlinking and token revocation

Unlinking removes Vutame's stored AT OAuth credentials and the local DID mapping. It does **not** delete records already present in the user's PDS.

Users who want to revoke Vutame's authorization should also use the authorization server/PDS revocation controls made available by their AT Protocol provider. Vutame cannot guarantee remote token revocation merely by deleting its local encrypted copy.

If the user wants portable records removed, they should delete them from the PDS or explicitly synchronize their desired state before unlinking.

## Export and migration

The DID and PDS-hosted Vutame records are portable independently of the Vutame account. Another compatible application can read the published Lexicons from the user's repository.

Vutame's normal JSON export remains the migration path for centralized Vutame-only data. Exports intentionally omit secrets, token hashes, raw OAuth credentials, DPoP private keys, and analytics visitor identifiers.

## Interoperability expectations

A compatible implementation should:

- treat the DID as canonical and handles as mutable;
- support the published Lexicon schema rather than depending on Vutame database columns;
- accept record replay idempotently;
- honor record deletion;
- avoid trusting the portable `verified` property as an independent proof of identity;
- preserve unknown future-compatible fields when its own storage model allows it.

Vutame's test suite covers OAuth → publish → Jetstream ingest → AppView render-model behavior against AT-compatible OAuth/XRPC fixtures, plus SQLite and Turso-engine persistence paths.
