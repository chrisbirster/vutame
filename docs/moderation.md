# Moderation operations

Vutame separates creator-facing safety controls from privileged moderation operations. Creators can block, mute, report, manage privacy, and appeal decisions without payment. Moderator actions require a normal authenticated Vutame session plus a durable server-side role.

## Authorization model

There is no shared admin password, query parameter, API key, or special HTTP header for moderation.

A moderator must:

1. have a normal row in `users`,
2. authenticate through the ordinary Vutame session flow, and
3. have a row in `moderation_admins` with role `moderator` or `admin`.

The application checks the role on every privileged moderation request. Removing the row removes privileged access without invalidating the person's ordinary Vutame account.

### Provisioning a moderator

Role provisioning is an operational database action, not a public API. First confirm the intended account exists, then apply a controlled statement such as:

```sql
INSERT INTO moderation_admins (user_id, role, created_at, updated_at)
SELECT id, 'moderator',
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM users
WHERE email = 'moderator@example.com';
```

Use `admin` only for operators who need the broader administrative role. M6 currently gives `moderator` and `admin` the same moderation HTTP permissions so later operational capabilities can distinguish them without replacing the role model.

To revoke moderation access:

```sql
DELETE FROM moderation_admins WHERE user_id = 'usr_...';
```

Do not provision roles from unreviewed user input or expose role mutation through the public application API.

## Report workflow

Reports begin in `open`. A moderator can assign a report to themselves, which moves it to `reviewing`. The report can then be resolved or dismissed.

Operational endpoints:

```text
GET  /api/v1/admin/moderation/session
GET  /api/v1/admin/moderation/reports
POST /api/v1/admin/moderation/reports/{id}/assign
POST /api/v1/admin/moderation/reports/{id}/notes
POST /api/v1/admin/moderation/reports/{id}/resolve
PUT  /api/v1/admin/moderation/profiles/{handle}
GET  /api/v1/admin/moderation/actions
GET  /api/v1/admin/moderation/abuse
GET  /api/v1/admin/moderation/appeals
POST /api/v1/admin/moderation/appeals/{id}/review
```

Assignment metadata lives in `report_workflow`. Human decisions and notes are appended to `moderation_actions`; that table is an audit log and is not used as mutable current state.

## Profile moderation states

The existing `moderation_profiles` table remains the current-state source for `active`, `restricted`, and `suspended`.

- `active` — normal product behavior.
- `restricted` — the profile remains directly addressable, but discovery and activity surfaces omit it while the restriction is active.
- `suspended` — public creator/social surfaces suppress the profile.
- `takedown` — an operator command represented durably as `suspended` plus an active `content_takedowns` record. This distinguishes a deliberate content takedown from other suspension reasons and preserves an audit reference.
- `restore` — returns the moderation state to `active` and deactivates any current takedown row.

A takedown is applied transactionally with its audit action so Vutame does not record an enforcement decision without changing the corresponding current state.

## Public propagation

For a local Vutame identity, suspension/takedown suppresses:

- `GET /api/v1/profiles/{handle}`
- server-rendered `/@handle` HTML
- verified custom-domain rendering
- Vutame discovery and social search
- activity/feed visibility
- public contact-capture block reads and new contact submissions
- direct AppView rendering of a locally linked AT Protocol DID

Existing safety guards continue to apply blocks, mutes, privacy settings, and follow policy independently.

A moderation takedown does **not** delete a creator's account, export data, handle ownership, or records already owned by the creator's AT Protocol PDS. Vutame controls what Vutame serves and indexes; it does not convert a moderation decision into ownership of portable identity data.

Remote AppView-only identities that are not linked to a local Vutame profile are not affected by a local profile action because there is no local moderation target row for them.

## Appeals

Appeals are part of the free safety layer:

```text
GET  /api/v1/me/moderation/appeals
POST /api/v1/me/moderation/appeals
```

A creator must have claimed a Vuta, but no paid entitlement is required. Appeal creation is rate limited. Moderators can move an appeal through `reviewing`, `resolved`, or `dismissed` and attach a response note. Appeal review is also written to the moderation action ledger.

Resolving an appeal does not automatically restore a profile. If restoration is appropriate, the moderator explicitly performs the `active`/restore action so the audit trail shows both the appeal decision and the enforcement-state change.

## Abuse counters

`GET /api/v1/admin/moderation/abuse` returns aggregate operational counts for a bounded time window, including report volume by reason, queue depth, active restrictions/suspensions/takedowns, and open appeals.

These counters deliberately avoid visitor identifiers, IP addresses, raw request bodies, or other unnecessary user-level telemetry. Detailed report content is available only through the authenticated moderation workflow.

## Operational checklist

For a report requiring enforcement:

1. Open the moderation queue and assign the report.
2. Review the report detail and relevant public content.
3. Add an internal note when context is needed for another operator.
4. Apply `restricted`, `suspended`, or `takedown` only when policy requires it.
5. Resolve or dismiss the report with a concise reason.
6. Verify the action appears in the audit ledger.
7. For takedowns, verify the public profile/API/custom-domain surface returns not found and that discovery no longer lists the creator.
8. If the creator appeals, review the appeal independently and use an explicit restore action when reversing enforcement.

## Data retention and privacy

Moderation records should be retained only as long as necessary for safety, legal, and abuse-prevention purposes. M6 Slice 4 consolidates repository privacy/retention policy; until then, operators should avoid copying report or appeal content into external systems unless incident handling requires it.

Never place passwords, session tokens, billing secrets, OAuth tokens, or raw webhook payloads in moderation notes.
