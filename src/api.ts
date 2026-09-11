export type Link = {
  id: string;
  label: string;
  url: string;
  kind: string;
  position: number;
  is_active: boolean;
};

export type Profile = {
  id: string;
  handle: string;
  display_name: string;
  bio: string;
  avatar_url?: string;
  verified: boolean;
  atproto_did?: string;
  links: Link[];
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
  if (!response.ok) throw new Error(`profile request failed: ${response.status}`);
  return response.json() as Promise<Profile>;
}

export async function fetchDiscover(): Promise<Profile[]> {
  const response = await fetch("/api/v1/discover");
  if (!response.ok) throw new Error(`discover request failed: ${response.status}`);
  const payload = (await response.json()) as { profiles: Profile[] };
  return payload.profiles;
}

export async function fetchAppMeta(): Promise<AppMeta> {
  const response = await fetch("/api/v1/meta");
  if (!response.ok) throw new Error(`meta request failed: ${response.status}`);
  return response.json() as Promise<AppMeta>;
}

export async function fetchHandleAvailability(handle: string): Promise<HandleAvailability> {
  const response = await fetch(`/api/v1/handles/${encodeURIComponent(handle)}/availability`);
  if (!response.ok) throw new Error(`handle availability request failed: ${response.status}`);
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

async function authJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw await apiError(response, "authentication request failed");
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
