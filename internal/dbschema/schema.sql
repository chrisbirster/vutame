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
  referrer_host TEXT NOT NULL DEFAULT '',
  device_class TEXT NOT NULL DEFAULT 'unknown' CHECK (device_class IN ('desktop', 'mobile', 'tablet', 'unknown')),
  created_at TEXT NOT NULL
);

CREATE INDEX analytics_events_user_created_idx ON analytics_events(user_id, created_at DESC, id DESC);
CREATE INDEX analytics_events_link_created_idx ON analytics_events(link_id, created_at DESC, id DESC);
CREATE INDEX analytics_events_kind_created_idx ON analytics_events(kind, created_at DESC, id DESC);
CREATE INDEX analytics_events_user_kind_visitor_idx ON analytics_events(user_id, kind, visitor_hash, created_at DESC);

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
