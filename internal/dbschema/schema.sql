CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL COLLATE NOCASE UNIQUE,
  email_verified INTEGER NOT NULL DEFAULT 0 CHECK (email_verified IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE profiles (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  handle TEXT NOT NULL COLLATE NOCASE UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  bio TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  theme TEXT NOT NULL DEFAULT 'midnight',
  verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
  atproto_did TEXT UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE creator_metadata (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  category TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE creator_interests (
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  interest TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY (user_id, interest)
);
CREATE INDEX creator_interests_interest_idx ON creator_interests(interest, user_id);

CREATE TABLE creator_privacy (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  discoverable INTEGER NOT NULL DEFAULT 1 CHECK (discoverable IN (0, 1)),
  activity_visible INTEGER NOT NULL DEFAULT 1 CHECK (activity_visible IN (0, 1)),
  allow_follows INTEGER NOT NULL DEFAULT 1 CHECK (allow_follows IN (0, 1)),
  updated_at TEXT NOT NULL
);

CREATE TABLE moderation_profiles (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'restricted', 'suspended')),
  note TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE follows (
  follower_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  following_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (follower_user_id, following_user_id),
  CHECK (follower_user_id <> following_user_id)
);
CREATE INDEX follows_follower_created_idx ON follows(follower_user_id, created_at DESC, following_user_id);
CREATE INDEX follows_following_created_idx ON follows(following_user_id, created_at DESC, follower_user_id);

CREATE TABLE blocks (
  blocker_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  blocked_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (blocker_user_id, blocked_user_id),
  CHECK (blocker_user_id <> blocked_user_id)
);
CREATE INDEX blocks_blocked_idx ON blocks(blocked_user_id, blocker_user_id);

CREATE TABLE mutes (
  muter_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  muted_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (muter_user_id, muted_user_id),
  CHECK (muter_user_id <> muted_user_id)
);
CREATE INDEX mutes_muted_idx ON mutes(muted_user_id, muter_user_id);

CREATE TABLE reports (
  id TEXT PRIMARY KEY,
  reporter_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  reported_user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  reason TEXT NOT NULL CHECK (reason IN ('spam', 'harassment', 'impersonation', 'unsafe', 'other')),
  detail TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewing', 'resolved', 'dismissed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (reporter_user_id <> reported_user_id)
);
CREATE INDEX reports_status_created_idx ON reports(status, created_at DESC, id DESC);
CREATE INDEX reports_reported_created_idx ON reports(reported_user_id, created_at DESC, id DESC);

CREATE TABLE moderation_admins (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('moderator', 'admin')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX moderation_admins_role_idx ON moderation_admins(role, user_id);

CREATE TABLE report_workflow (
  report_id TEXT PRIMARY KEY REFERENCES reports(id) ON DELETE CASCADE,
  assigned_admin_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  resolved_at TEXT,
  updated_at TEXT NOT NULL
);
CREATE INDEX report_workflow_assignee_idx ON report_workflow(assigned_admin_user_id, updated_at DESC);

CREATE TABLE moderation_actions (
  id TEXT PRIMARY KEY,
  actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  actor_role TEXT NOT NULL CHECK (actor_role IN ('moderator', 'admin')),
  target_user_id TEXT REFERENCES profiles(user_id) ON DELETE SET NULL,
  report_id TEXT REFERENCES reports(id) ON DELETE SET NULL,
  action TEXT NOT NULL CHECK (action IN ('assign', 'note', 'restrict', 'suspend', 'takedown', 'restore', 'resolve', 'dismiss', 'appeal_review', 'appeal_resolve')),
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX moderation_actions_created_idx ON moderation_actions(created_at DESC, id DESC);
CREATE INDEX moderation_actions_target_idx ON moderation_actions(target_user_id, created_at DESC, id DESC);
CREATE INDEX moderation_actions_report_idx ON moderation_actions(report_id, created_at DESC, id DESC);

CREATE TABLE content_takedowns (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  reason TEXT NOT NULL DEFAULT '',
  action_id TEXT REFERENCES moderation_actions(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX content_takedowns_active_idx ON content_takedowns(active, updated_at DESC);

CREATE TABLE moderation_appeals (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  message TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewing', 'resolved', 'dismissed')),
  assigned_admin_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  response_note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX moderation_appeals_user_idx ON moderation_appeals(user_id, created_at DESC, id DESC);
CREATE INDEX moderation_appeals_status_idx ON moderation_appeals(status, created_at DESC, id DESC);

CREATE TABLE links (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  url TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'website',
  thumbnail_url TEXT NOT NULL DEFAULT '',
  featured INTEGER NOT NULL DEFAULT 0 CHECK (featured IN (0, 1)),
  visible_from TEXT,
  visible_until TEXT,
  position INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX links_user_position_idx ON links(user_id, position, id);

CREATE TABLE activity_events (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('profile_updated', 'link_featured')),
  link_id TEXT REFERENCES links(id) ON DELETE SET NULL,
  label TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX activity_events_created_idx ON activity_events(created_at DESC, id DESC);
CREATE INDEX activity_events_user_created_idx ON activity_events(user_id, created_at DESC, id DESC);

CREATE TABLE analytics_events (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  link_id TEXT REFERENCES links(id) ON DELETE SET NULL,
  kind TEXT NOT NULL CHECK (kind IN ('profile_view', 'link_click')),
  visitor_hash TEXT NOT NULL DEFAULT '',
  campaign TEXT NOT NULL DEFAULT '',
  referrer_host TEXT NOT NULL DEFAULT '',
  device_class TEXT NOT NULL DEFAULT 'unknown' CHECK (device_class IN ('desktop', 'mobile', 'tablet', 'unknown')),
  created_at TEXT NOT NULL
);
CREATE INDEX analytics_events_user_created_idx ON analytics_events(user_id, created_at DESC, id DESC);
CREATE INDEX analytics_events_link_created_idx ON analytics_events(link_id, created_at DESC, id DESC);
CREATE INDEX analytics_events_kind_created_idx ON analytics_events(kind, created_at DESC, id DESC);
CREATE INDEX analytics_events_user_kind_visitor_idx ON analytics_events(user_id, kind, visitor_hash, created_at DESC);
CREATE INDEX analytics_events_user_campaign_idx ON analytics_events(user_id, campaign, created_at DESC);

CREATE TABLE creator_data_settings (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  analytics_retention_days INTEGER NOT NULL DEFAULT 90 CHECK (analytics_retention_days IN (30, 90, 365)),
  contact_retention_days INTEGER NOT NULL DEFAULT 365 CHECK (contact_retention_days IN (30, 90, 365)),
  updated_at TEXT NOT NULL
);

CREATE TABLE contact_blocks (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
  heading TEXT NOT NULL DEFAULT 'Stay in touch',
  description TEXT NOT NULL DEFAULT '',
  consent_text TEXT NOT NULL DEFAULT 'I agree to share my email with this creator for the purpose described above.',
  button_label TEXT NOT NULL DEFAULT 'Sign up',
  updated_at TEXT NOT NULL
);

CREATE TABLE contact_submissions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  email TEXT NOT NULL COLLATE NOCASE,
  consent_text TEXT NOT NULL,
  campaign TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX contact_submissions_user_created_idx ON contact_submissions(user_id, created_at DESC, id DESC);
CREATE INDEX contact_submissions_user_email_idx ON contact_submissions(user_id, email, created_at DESC);

CREATE TABLE custom_domains (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  hostname TEXT NOT NULL COLLATE NOCASE UNIQUE,
  verification_token TEXT NOT NULL,
  verified_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(user_id, hostname)
);
CREATE INDEX custom_domains_user_idx ON custom_domains(user_id, created_at DESC);
CREATE INDEX custom_domains_verified_idx ON custom_domains(hostname, verified_at);

CREATE TABLE verification_requests (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  method TEXT NOT NULL CHECK (method IN ('custom_domain', 'atproto')),
  evidence TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX verification_requests_user_idx ON verification_requests(user_id, created_at DESC);
CREATE INDEX verification_requests_status_idx ON verification_requests(status, created_at DESC);

CREATE TABLE api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  scopes TEXT NOT NULL,
  expires_at TEXT,
  last_used_at TEXT,
  revoked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX api_tokens_user_idx ON api_tokens(user_id, created_at DESC);
CREATE INDEX api_tokens_hash_idx ON api_tokens(token_hash);

CREATE TABLE webhooks (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  events TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX webhooks_user_idx ON webhooks(user_id, created_at DESC);

CREATE TABLE webhook_deliveries (
  id TEXT PRIMARY KEY,
  webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  event TEXT NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  response_code INTEGER,
  next_attempt_at TEXT NOT NULL,
  delivered_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX webhook_deliveries_pending_idx ON webhook_deliveries(status, next_attempt_at, created_at);
CREATE INDEX webhook_deliveries_user_idx ON webhook_deliveries(user_id, created_at DESC);

CREATE TABLE atproto_oauth_states (
  state_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  identifier TEXT NOT NULL,
  expected_did TEXT NOT NULL,
  pds_url TEXT NOT NULL,
  issuer TEXT NOT NULL,
  token_endpoint TEXT NOT NULL,
  verifier_enc TEXT NOT NULL,
  dpop_key_enc TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX atproto_oauth_states_expires_idx ON atproto_oauth_states(expires_at);

CREATE TABLE atproto_accounts (
  user_id TEXT PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  did TEXT NOT NULL UNIQUE,
  handle TEXT NOT NULL DEFAULT '',
  pds_url TEXT NOT NULL,
  issuer TEXT NOT NULL,
  token_endpoint TEXT NOT NULL,
  access_token_enc TEXT NOT NULL,
  refresh_token_enc TEXT NOT NULL,
  dpop_key_enc TEXT NOT NULL,
  scope TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  conflict_policy TEXT NOT NULL DEFAULT 'vutame_wins' CHECK (conflict_policy IN ('vutame_wins', 'pds_wins')),
  publish_enabled INTEGER NOT NULL DEFAULT 0 CHECK (publish_enabled IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX atproto_accounts_did_idx ON atproto_accounts(did);
CREATE INDEX atproto_accounts_handle_idx ON atproto_accounts(handle COLLATE NOCASE);

CREATE TABLE atproto_records (
  user_id TEXT NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  collection TEXT NOT NULL,
  rkey TEXT NOT NULL,
  cid TEXT NOT NULL DEFAULT '',
  local_updated_at TEXT NOT NULL DEFAULT '',
  synced_at TEXT NOT NULL,
  payload TEXT NOT NULL,
  PRIMARY KEY (user_id, collection, rkey)
);
CREATE INDEX atproto_records_user_synced_idx ON atproto_records(user_id, synced_at DESC);

CREATE TABLE atproto_indexed_profiles (
  did TEXT PRIMARY KEY,
  handle TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  bio TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  theme TEXT NOT NULL DEFAULT 'midnight',
  verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
  record_json TEXT NOT NULL,
  indexed_at TEXT NOT NULL
);
CREATE INDEX atproto_indexed_profiles_handle_idx ON atproto_indexed_profiles(handle COLLATE NOCASE);
CREATE INDEX atproto_indexed_profiles_name_idx ON atproto_indexed_profiles(display_name COLLATE NOCASE);

CREATE TABLE atproto_indexed_links (
  did TEXT NOT NULL REFERENCES atproto_indexed_profiles(did) ON DELETE CASCADE,
  rkey TEXT NOT NULL,
  label TEXT NOT NULL,
  url TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'website',
  thumbnail_url TEXT NOT NULL DEFAULT '',
  featured INTEGER NOT NULL DEFAULT 0 CHECK (featured IN (0, 1)),
  position INTEGER NOT NULL DEFAULT 0,
  record_json TEXT NOT NULL,
  indexed_at TEXT NOT NULL,
  PRIMARY KEY (did, rkey)
);
CREATE INDEX atproto_indexed_links_did_position_idx ON atproto_indexed_links(did, position, rkey);

CREATE TABLE atproto_jetstream_state (
  name TEXT PRIMARY KEY,
  cursor_us INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);

CREATE TABLE billing_customers (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT 'stripe' CHECK (provider IN ('stripe')),
  customer_id TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX billing_customers_customer_idx ON billing_customers(customer_id);

CREATE TABLE billing_subscriptions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT 'stripe' CHECK (provider IN ('stripe')),
  provider_subscription_id TEXT NOT NULL UNIQUE,
  customer_id TEXT NOT NULL,
  price_id TEXT NOT NULL,
  plan TEXT NOT NULL CHECK (plan IN ('pro')),
  status TEXT NOT NULL CHECK (status IN ('incomplete','incomplete_expired','trialing','active','past_due','canceled','unpaid','paused')),
  current_period_end TEXT,
  cancel_at_period_end INTEGER NOT NULL DEFAULT 0 CHECK (cancel_at_period_end IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX billing_subscriptions_user_status_idx ON billing_subscriptions(user_id, status, updated_at DESC);
CREATE INDEX billing_subscriptions_customer_idx ON billing_subscriptions(customer_id, updated_at DESC);

CREATE TABLE billing_events (
  provider_event_id TEXT PRIMARY KEY,
  provider TEXT NOT NULL DEFAULT 'stripe' CHECK (provider IN ('stripe')),
  event_type TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  received_at TEXT NOT NULL,
  processed_at TEXT NOT NULL
);
CREATE INDEX billing_events_processed_idx ON billing_events(processed_at DESC);

CREATE TABLE entitlement_grants (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  feature TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('admin', 'migration')),
  expires_at TEXT,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (user_id, feature)
);
CREATE INDEX entitlement_grants_expires_idx ON entitlement_grants(expires_at, user_id);

CREATE TABLE media_assets (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slot TEXT NOT NULL CHECK (slot IN ('avatar')),
  content_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
  storage_key TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(user_id, slot)
);
CREATE INDEX media_assets_user_idx ON media_assets(user_id);

CREATE TABLE auth_challenges (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL COLLATE NOCASE,
  code_hash TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX auth_challenges_email_idx ON auth_challenges(email, created_at);
CREATE INDEX auth_challenges_expires_idx ON auth_challenges(expires_at);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_expires_idx ON sessions(expires_at);
