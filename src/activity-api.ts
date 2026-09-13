export type ActivityEvent = {
  id: string;
  kind: "profile_updated" | "link_featured";
  handle: string;
  display_name: string;
  avatar_url?: string;
  link_id?: string;
  label?: string;
  created_at: string;
};

export type ActivityPage = {
  events: ActivityEvent[];
  next_cursor?: string;
};

export type TrendingCreator = {
  handle: string;
  display_name: string;
  avatar_url?: string;
  recent_activity: number;
  follower_count: number;
  score: number;
};

export async function fetchFollowingActivity(cursor = "", limit = 20): Promise<ActivityPage> {
  return fetchActivityPage("/api/v1/feed", cursor, limit);
}

export async function fetchRecentActivity(cursor = "", limit = 20): Promise<ActivityPage> {
  return fetchActivityPage("/api/v1/activity/recent", cursor, limit);
}

export async function fetchTrendingCreators(limit = 10): Promise<TrendingCreator[]> {
  const response = await fetch(`/api/v1/discovery/trending?limit=${limit}`);
  if (!response.ok) throw await activityError(response, "trending creators unavailable");
  const payload = (await response.json()) as { creators: TrendingCreator[] };
  return payload.creators;
}

async function fetchActivityPage(path: string, cursor: string, limit: number): Promise<ActivityPage> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cursor) params.set("cursor", cursor);
  const response = await fetch(`${path}?${params.toString()}`, { credentials: "same-origin" });
  if (!response.ok) throw await activityError(response, "activity unavailable");
  return response.json() as Promise<ActivityPage>;
}

async function activityError(response: Response, fallback: string) {
  try {
    const payload = (await response.json()) as { error?: string };
    return new Error(payload.error || fallback);
  } catch {
    return new Error(fallback);
  }
}
