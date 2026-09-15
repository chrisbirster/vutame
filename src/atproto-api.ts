export type ATProtoIdentity = {
  did: string;
  handle?: string;
  pds_url: string;
};

export type ATProtoAccount = {
  did: string;
  handle?: string;
  pds_url: string;
  scope: string;
  conflict_policy: "vutame_wins" | "pds_wins";
  publish_enabled: boolean;
  expires_at: string;
  updated_at: string;
};

export type ATProtoAccountState =
  | { linked: false }
  | { linked: true; account: ATProtoAccount };

export type ATProtoOAuthStart = {
  authorization_url: string;
  did: string;
  handle?: string;
};

export type ATProtoSyncedRecord = {
  collection: string;
  rkey: string;
  cid?: string;
  local_updated_at?: string;
  synced_at: string;
};

export type ATProtoSyncStatus = {
  account: ATProtoAccount;
  records: ATProtoSyncedRecord[];
};

export type ATProtoSyncConflict = {
  collection: string;
  rkey: string;
  reason: string;
};

export type ATProtoSyncReport = {
  did: string;
  published: number;
  deleted: number;
  conflicts: ATProtoSyncConflict[];
  synced_at: string;
};

export type PortableLink = {
  rkey: string;
  label: string;
  url: string;
  kind: string;
  thumbnail_url?: string;
  featured: boolean;
  position: number;
};

export type PortableProfile = {
  did: string;
  handle?: string;
  display_name?: string;
  bio?: string;
  avatar_url?: string;
  theme: string;
  verified: boolean;
  links: PortableLink[];
  indexed_at: string;
};

export type PortableProfileResponse = {
  profile: PortableProfile;
  identity?: ATProtoIdentity;
};

export async function resolveATProtoIdentity(identifier: string): Promise<ATProtoIdentity> {
  const params = new URLSearchParams({ identifier: identifier.trim() });
  return requestJSON<ATProtoIdentity>(`/api/v1/atproto/resolve?${params.toString()}`);
}

export async function fetchPortableProfile(did: string): Promise<PortableProfileResponse> {
  return requestJSON<PortableProfileResponse>(`/api/v1/atproto/profiles/${encodeURIComponent(did)}`);
}

export async function searchPortableProfiles(query = "", limit = 20): Promise<PortableProfile[]> {
  const params = new URLSearchParams();
  if (query.trim()) params.set("q", query.trim());
  params.set("limit", String(limit));
  const payload = await requestJSON<{ profiles: PortableProfile[] }>(`/api/v1/atproto/search?${params.toString()}`);
  return payload.profiles;
}

export async function fetchATProtoAccount(): Promise<ATProtoAccountState> {
  return requestJSON<ATProtoAccountState>("/api/v1/me/atproto", { credentials: "same-origin" });
}

export async function fetchATProtoSyncStatus(): Promise<ATProtoSyncStatus> {
  return requestJSON<ATProtoSyncStatus>("/api/v1/me/atproto/status", { credentials: "same-origin" });
}

export async function syncATProto(): Promise<ATProtoSyncReport> {
  return requestJSON<ATProtoSyncReport>("/api/v1/me/atproto/sync", {
    method: "POST",
    credentials: "same-origin",
  });
}

export async function startATProtoOAuth(identifier: string): Promise<ATProtoOAuthStart> {
  return requestJSON<ATProtoOAuthStart>("/api/v1/me/atproto/oauth/start", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ identifier }),
  });
}

export async function updateATProtoSettings(input: {
  conflict_policy: "vutame_wins" | "pds_wins";
  publish_enabled: boolean;
}): Promise<ATProtoAccount> {
  return requestJSON<ATProtoAccount>("/api/v1/me/atproto", {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function unlinkATProto(): Promise<void> {
  const response = await fetch("/api/v1/me/atproto", {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!response.ok) throw await responseError(response, "unlink failed");
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  if (!response.ok) throw await responseError(response, "AT Protocol request failed");
  return response.json() as Promise<T>;
}

async function responseError(response: Response, fallback: string): Promise<Error> {
  try {
    const payload = (await response.json()) as { error?: string };
    return new Error(payload.error || fallback);
  } catch {
    return new Error(fallback);
  }
}
