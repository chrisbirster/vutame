import type { Creator } from "./api";

export type CreatorPage = {
  creators: Creator[];
  next_cursor?: string;
};

export async function fetchDiscoveryPage(input: {
  q?: string;
  category?: string;
  interest?: string;
  cursor?: string;
  limit?: number;
} = {}): Promise<CreatorPage> {
  const params = new URLSearchParams();
  if (input.q?.trim()) params.set("q", input.q.trim());
  if (input.category?.trim()) params.set("category", input.category.trim());
  if (input.interest?.trim()) params.set("interest", input.interest.trim());
  if (input.cursor?.trim()) params.set("cursor", input.cursor.trim());
  if (input.limit) params.set("limit", String(input.limit));
  const suffix = params.size ? `?${params.toString()}` : "";
  const response = await fetch(`/api/v1/discovery${suffix}`, { credentials: "same-origin" });
  if (!response.ok) {
    let message = "creator discovery failed";
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) message = payload.error;
    } catch {
      // Keep the stable fallback message.
    }
    throw new Error(message);
  }
  return response.json() as Promise<CreatorPage>;
}
