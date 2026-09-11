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
