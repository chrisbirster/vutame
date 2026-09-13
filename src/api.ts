import type { LinkKind } from "./link-kinds";

export type Link = {
  id: string;
  label: string;
  url: string;
  kind: LinkKind;
  thumbnail_url?: string;
  featured: boolean;
  visible_from?: string;
  visible_until?: string;
  position: number;
  is_active: boolean;
};

export type Profile = {
  id: string;
  handle: string;
  display_name: string;
  bio: string;
  avatar_url?: string;
  theme: string;
  verified: boolean;
  atproto_did?: string;
  links: Link[];
};

export type Creator = {
  handle: string;
  display_name: string;
  bio: string;
  avatar_url?: string;
  theme: string;
  verified: boolean;
  category?: string;
  interests: string[];
  follower_count: number;
  following_count: number;
  viewer_follows: boolean;
};

export type DiscoveryMetadataInput = {
  category: string;
  interests: string[];
};

export type ProfileUpdate = {
  display_name: string;
  bio: string;
  avatar_url: string;
  theme: string;
};

export type LinkInput = {
  label: string;
  url: string;
  kind: LinkKind;
  thumbnail_url: string;
  featured: boolean;
  visible_from: string;
  visible_until: string;
  is_active: boolean;
};

export type MediaAsset = {
  id: string;
  url: string;
  content_type: string;
  size_bytes: number;
};

export type LinkPreviewMetadata = {
  url: string;
  title?: string;
  description?: string;
  image_url?: string;
  site_name?: string;
  provider: string;
};

export type AppMeta = {
  product: string;
  marketing_origin: string;
  profile_origin: string;
};

export type HandleAvailability = {
  handle: string;
  available: boolean;
};

export type AuthUser = {
  id: string;
  email: string;
  email_verified: boolean;
};

export type AuthSession =
  | { authenticated: false }
  | { authenticated: true; user: AuthUser };

export class APIError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "APIError";
    this.status = status;
  }
}

export async function fetchProfile(handle: string): Promise<Profile | null> {
  const response = await fetch(`/api/v1/profiles/${encodeURIComponent(handle)}`);
  if (response.status === 404) return null;
  if (!response.ok) throw await apiError(response, "profile request failed");
  return response.json() as Promise<Profile>;
}

export async function fetchDiscover(): Promise<Profile[]> {
  const response = await fetch("/api/v1/discover");
  if (!response.ok) throw await apiError(response, "discover request failed");
  const payload = (await response.json()) as { profiles: Profile[] };
  return payload.profiles;
}

export async function searchCreators(input: {
  q?: string;
  category?: string;
  interest?: string;
  limit?: number;
} = {}): Promise<Creator[]> {
  const params = new URLSearchParams();
  if (input.q?.trim()) params.set("q", input.q.trim());
  if (input.category?.trim()) params.set("category", input.category.trim());
  if (input.interest?.trim()) params.set("interest", input.interest.trim());
  if (input.limit) params.set("limit", String(input.limit));
  const suffix = params.size ? `?${params.toString()}` : "";
  const response = await fetch(`/api/v1/discovery${suffix}`, { credentials: "same-origin" });
  if (!response.ok) throw await apiError(response, "creator discovery failed");
  const payload = (await response.json()) as { creators: Creator[] };
  return payload.creators;
}

export async function fetchCreatorSocial(handle: string): Promise<Creator | null> {
  const response = await fetch(`/api/v1/creators/${encodeURIComponent(handle)}/social`, { credentials: "same-origin" });
  if (response.status === 404 || response.status === 503) return null;
  if (!response.ok) throw await apiError(response, "creator social request failed");
  return response.json() as Promise<Creator>;
}

export async function fetchFollowers(handle: string): Promise<Creator[]> {
  return creatorList(`/api/v1/creators/${encodeURIComponent(handle)}/followers`);
}

export async function fetchFollowing(handle: string): Promise<Creator[]> {
  return creatorList(`/api/v1/creators/${encodeURIComponent(handle)}/following`);
}

export async function followCreator(handle: string): Promise<void> {
  return socialMutation("PUT", `/api/v1/me/follows/${encodeURIComponent(handle)}`, "follow failed");
}

export async function unfollowCreator(handle: string): Promise<void> {
  return socialMutation("DELETE", `/api/v1/me/follows/${encodeURIComponent(handle)}`, "unfollow failed");
}

export async function updateDiscoveryProfile(input: DiscoveryMetadataInput): Promise<Creator> {
  return mutationJSON<Creator>("PUT", "/api/v1/me/discovery-profile", input, "discovery profile update failed");
}

export async function fetchAppMeta(): Promise<AppMeta> {
  const response = await fetch("/api/v1/meta");
  if (!response.ok) throw await apiError(response, "meta request failed");
  return response.json() as Promise<AppMeta>;
}

export async function fetchHandleAvailability(handle: string): Promise<HandleAvailability> {
  const response = await fetch(`/api/v1/handles/${encodeURIComponent(handle)}/availability`);
  if (!response.ok) throw await apiError(response, "handle availability request failed");
  return response.json() as Promise<HandleAvailability>;
}

export async function requestAuthCode(email: string): Promise<{ challenge_id: string }> {
  return authJSON("/api/v1/auth/code", { email });
}

export async function verifyAuthCode(
  challengeId: string,
  email: string,
  code: string,
): Promise<{ user: AuthUser }> {
  return authJSON("/api/v1/auth/verify", {
    challenge_id: challengeId,
    email,
    code,
  });
}

export async function fetchAuthSession(): Promise<AuthSession> {
  const response = await fetch("/api/v1/auth/session", { credentials: "same-origin" });
  if (!response.ok) throw await apiError(response, "session request failed");
  return response.json() as Promise<AuthSession>;
}

export async function logoutAuth(): Promise<void> {
  const response = await fetch("/api/v1/auth/logout", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  if (!response.ok) throw await apiError(response, "logout failed");
}

export async function fetchOwnedProfile(): Promise<Profile | null> {
  const response = await fetch("/api/v1/me/profile", { credentials: "same-origin" });
  if (response.status === 404) return null;
  if (!response.ok) throw await apiError(response, "profile editor request failed");
  return response.json() as Promise<Profile>;
}

export async function claimOwnedProfile(handle: string): Promise<Profile> {
  return mutationJSON<Profile>("POST", "/api/v1/me/profile/claim", { handle });
}

export async function updateOwnedProfile(input: ProfileUpdate): Promise<Profile> {
  return mutationJSON<Profile>("PATCH", "/api/v1/me/profile", input);
}

export async function uploadOwnedAvatar(file: File): Promise<MediaAsset> {
  const body = new FormData();
  body.set("file", file);
  const response = await fetch("/api/v1/me/avatar", {
    method: "POST",
    credentials: "same-origin",
    body,
  });
  if (!response.ok) throw await apiError(response, "avatar upload failed");
  return response.json() as Promise<MediaAsset>;
}

export async function deleteOwnedAvatar(): Promise<void> {
  const response = await fetch("/api/v1/me/avatar", {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!response.ok) throw await apiError(response, "avatar delete failed");
}

export async function fetchLinkPreview(url: string): Promise<LinkPreviewMetadata> {
  return mutationJSON<LinkPreviewMetadata>("POST", "/api/v1/me/link-preview", { url }, "preview metadata unavailable");
}

export async function createOwnedLink(input: LinkInput): Promise<Link> {
  return mutationJSON<Link>("POST", "/api/v1/me/links", input);
}

export async function updateOwnedLink(id: string, input: LinkInput): Promise<Link> {
  return mutationJSON<Link>("PATCH", `/api/v1/me/links/${encodeURIComponent(id)}`, input);
}

export async function deleteOwnedLink(id: string): Promise<void> {
  const response = await fetch(`/api/v1/me/links/${encodeURIComponent(id)}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!response.ok) throw await apiError(response, "delete link failed");
}

export async function reorderOwnedLinks(ids: string[]): Promise<void> {
  const response = await fetch("/api/v1/me/links/order", {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
  if (!response.ok) throw await apiError(response, "reorder links failed");
}

async function creatorList(path: string): Promise<Creator[]> {
  const response = await fetch(path, { credentials: "same-origin" });
  if (!response.ok) throw await apiError(response, "creator list request failed");
  const payload = (await response.json()) as { creators: Creator[] };
  return payload.creators;
}

async function socialMutation(method: string, path: string, fallback: string): Promise<void> {
  const response = await fetch(path, { method, credentials: "same-origin" });
  if (!response.ok) throw await apiError(response, fallback);
}

async function authJSON<T>(path: string, body: unknown): Promise<T> {
  return mutationJSON<T>("POST", path, body, "authentication request failed");
}

async function mutationJSON<T>(method: string, path: string, body: unknown, fallback = "request failed"): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw await apiError(response, fallback);
  return response.json() as Promise<T>;
}

async function apiError(response: Response, fallback: string): Promise<APIError> {
  try {
    const payload = (await response.json()) as { error?: string };
    return new APIError(payload.error || fallback, response.status);
  } catch {
    return new APIError(fallback, response.status);
  }
}
