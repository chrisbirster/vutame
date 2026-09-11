export type Link = {
  id: string;
  label: string;
  url: string;
  kind: string;
};

export type Profile = {
  handle: string;
  display_name: string;
  bio: string;
  avatar_url?: string;
  verified: boolean;
  links: Link[];
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
