# Billing and entitlements

Vutame billing is optional. Stripe provides hosted subscription checkout and account management, while Vutame owns the entitlement model and persists only the billing state needed to determine access.

## Product boundary

The free identity layer is never paywalled. Payment does not control a creator's handle, basic profile/link publishing, social graph, safety controls, export, or AT Protocol portability.

The initial Pro entitlement set is:

- custom domains
- analytics history beyond 30 days
- premium themes
- branding removal
- advanced API/integration features

Only the first two are enforced in M6 Slice 1. The remaining entries are durable entitlement names reserved for later UI/product work.

When Stripe is not configured, candidate paid features remain available. This is intentional for local development, self-hosted installs, and deployments upgrading from pre-M6 behavior. Enabling billing must not turn configuration absence into a feature outage.

Existing custom domains are never destructively disabled if an account later lacks Pro. The entitlement gate applies to creating/verifying new paid-domain capability, not to deleting historical user state.

## Runtime configuration

Stripe billing is enabled only when all three provider values are present:

```bash
VUTAME_STRIPE_SECRET_KEY='sk_live_...'
VUTAME_STRIPE_WEBHOOK_SECRET='whsec_...'
VUTAME_STRIPE_PRO_PRICE_ID='price_...'
```

Checkout/portal URLs default to the marketing origin and may be overridden:

```bash
VUTAME_STRIPE_SUCCESS_URL='https://vutame.com/billing?checkout=success'
VUTAME_STRIPE_CANCEL_URL='https://vutame.com/billing?checkout=cancel'
VUTAME_STRIPE_PORTAL_RETURN_URL='https://vutame.com/billing'
```

`VUTAME_STRIPE_API_BASE` exists for local integration tests. Production rejects any value other than `https://api.stripe.com`.

Partial provider configuration is a startup error. Secret key, webhook secret, and Pro price ID must be configured together.

## Stripe setup

1. Create the recurring Pro product/price in Stripe.
2. Configure the three required variables above.
3. Register the webhook endpoint:

```text
POST https://vutame.com/api/v1/billing/stripe/webhook
```

4. Subscribe the endpoint to at least:
   - `customer.subscription.created`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
5. Copy the endpoint signing secret into `VUTAME_STRIPE_WEBHOOK_SECRET`.
6. Deploy only after the Atlas schema containing the billing tables has been applied.

The application uses Stripe-hosted Checkout and Customer Portal sessions. Stripe API requests include idempotency keys.

## Webhook security and persistence

The webhook handler verifies the `Stripe-Signature` HMAC against the exact raw request bytes with a five-minute timestamp tolerance. The body is capped at 1 MiB.

Successful provider events are written to an idempotency ledger keyed by Stripe event ID. Re-delivery returns success with `duplicate=true` without applying the event twice.

Vutame does **not** persist the raw Stripe event body. The ledger stores only:

- provider event ID
- event type
- SHA-256 payload hash
- received/processed timestamps

Unknown Stripe prices fail closed: they may be recorded in the event ledger, but they cannot grant Pro.

## Data ownership

Billing rows reference `users(id)`, not `profiles(user_id)`. A person may have an account and subscription before claiming a public profile.

Provider identifiers remain separate from Vutame identifiers:

- `billing_customers.user_id` — Vutame account ID
- `billing_customers.customer_id` — Stripe customer ID
- `billing_subscriptions.provider_subscription_id` — Stripe subscription ID

Manual/migration entitlements live in `entitlement_grants` and are independent of Stripe. This allows migrations or administrative corrections without manufacturing provider state.

## HTTP API

Authenticated creator endpoints:

```text
GET  /api/v1/me/billing
POST /api/v1/me/billing/checkout
POST /api/v1/me/billing/portal
```

Stripe webhook endpoint:

```text
POST /api/v1/billing/stripe/webhook
```

Checkout and portal endpoints are rate limited. The webhook endpoint is authenticated by Stripe signature rather than a Vutame login session.

## Deployment smoke test

After applying the schema and configuring Stripe:

1. Sign in with an account that has not claimed a profile and confirm `GET /api/v1/me/billing` returns `plan: "free"`.
2. Start checkout from `/billing` and confirm Stripe Checkout is hosted on Stripe.
3. Complete a test subscription and confirm the webhook changes the account to `plan: "pro"`.
4. Replay the same Stripe event and confirm it is accepted as a duplicate without creating another ledger row.
5. Confirm a Pro account can create a new custom domain and request analytics beyond 30 days.
6. Confirm a free account receives `402 Payment Required` for those paid operations while ordinary profile/link/social/export/ATProto functionality remains available.
7. Open the Customer Portal and confirm the return URL points back to `/billing`.
8. Cancel the subscription and confirm subsequent subscription state removes Pro once the provider reports a non-active/non-trialing state.

## Incident notes

If Stripe is unavailable, existing public profiles and all free identity functionality must continue operating. Do not remove billing configuration as an emergency mechanism unless intentionally accepting the documented behavior that paid candidate features become unlocked while billing is disabled.

If webhook delivery is delayed, Stripe remains the source of provider subscription truth and Vutame may temporarily show stale entitlement state. Replaying the missing signed events is safe because processing is idempotent.

Before changing the configured Pro price, plan a migration for existing active subscriptions. An unrecognized price intentionally does not grant Pro.
