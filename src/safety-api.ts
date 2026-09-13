export type PrivacySettings = {
  discoverable: boolean;
  activity_visible: boolean;
  allow_follows: boolean;
};

export type SafetyRelation = {
  handle: string;
  display_name: string;
  avatar_url?: string;
};

export async function fetchPrivacy(): Promise<PrivacySettings> {
  return jsonRequest<PrivacySettings>("GET", "/api/v1/me/privacy");
}

export async function updatePrivacy(input: PrivacySettings): Promise<PrivacySettings> {
  return jsonRequest<PrivacySettings>("PUT", "/api/v1/me/privacy", input);
}

export async function fetchBlocks(): Promise<SafetyRelation[]> {
  const payload = await jsonRequest<{ creators: SafetyRelation[] }>("GET", "/api/v1/me/blocks");
  return payload.creators;
}

export async function fetchMutes(): Promise<SafetyRelation[]> {
  const payload = await jsonRequest<{ creators: SafetyRelation[] }>("GET", "/api/v1/me/mutes");
  return payload.creators;
}

export async function blockCreator(handle: string): Promise<void> {
  await emptyRequest("PUT", `/api/v1/me/blocks/${encodeURIComponent(cleanHandle(handle))}`);
}

export async function unblockCreator(handle: string): Promise<void> {
  await emptyRequest("DELETE", `/api/v1/me/blocks/${encodeURIComponent(cleanHandle(handle))}`);
}

export async function muteCreator(handle: string): Promise<void> {
  await emptyRequest("PUT", `/api/v1/me/mutes/${encodeURIComponent(cleanHandle(handle))}`);
}

export async function unmuteCreator(handle: string): Promise<void> {
  await emptyRequest("DELETE", `/api/v1/me/mutes/${encodeURIComponent(cleanHandle(handle))}`);
}

export async function reportCreator(handle: string, reason: string, detail: string): Promise<void> {
  await jsonRequest("POST", `/api/v1/me/reports/${encodeURIComponent(cleanHandle(handle))}`, { reason, detail });
}

function cleanHandle(handle: string) {
  return handle.trim().replace(/^@/, "").toLowerCase();
}

async function emptyRequest(method: string, path: string) {
  const response = await fetch(path, { method, credentials: "same-origin" });
  if (!response.ok) throw await requestError(response, "safety request failed");
}

async function jsonRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) throw await requestError(response, "safety request failed");
  return response.json() as Promise<T>;
}

async function requestError(response: Response, fallback: string): Promise<Error> {
  try {
    const payload = (await response.json()) as { error?: string };
    return new Error(payload.error || fallback);
  } catch {
    return new Error(fallback);
  }
}
