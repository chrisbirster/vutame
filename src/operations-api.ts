export type CustomDomain = {
  id: string;
  hostname: string;
  verification_token: string;
  verified_at?: string;
  dns_name: string;
  dns_value: string;
};

export type VerificationRequest = {
  id: string;
  method: string;
  evidence: string;
  status: "pending" | "approved" | "rejected";
  note?: string;
  created_at: string;
  updated_at: string;
};

export type APIToken = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
};

export type CreatedToken = APIToken & { token: string };

export type Webhook = {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
  updated_at: string;
};

export type CreatedWebhook = Webhook & { signing_secret: string };

export async function fetchDomains(): Promise<CustomDomain[]> {
  return getList<CustomDomain>("/api/v1/me/domains", "domains");
}

export async function addDomain(hostname: string): Promise<CustomDomain> {
  return mutation<CustomDomain>("POST", "/api/v1/me/domains", { hostname });
}

export async function verifyDomain(id: string): Promise<CustomDomain> {
  return mutation<CustomDomain>("POST", `/api/v1/me/domains/${encodeURIComponent(id)}/verify`, {});
}

export async function deleteDomain(id: string): Promise<void> {
  return noContent("DELETE", `/api/v1/me/domains/${encodeURIComponent(id)}`);
}

export async function fetchVerificationRequests(): Promise<VerificationRequest[]> {
  return getList<VerificationRequest>("/api/v1/me/verification", "requests");
}

export async function fetchTokens(): Promise<APIToken[]> {
  return getList<APIToken>("/api/v1/me/tokens", "tokens");
}

export async function createToken(input: { name: string; scopes: string[]; expires_in_days: number }): Promise<CreatedToken> {
  return mutation<CreatedToken>("POST", "/api/v1/me/tokens", input);
}

export async function revokeToken(id: string): Promise<void> {
  return noContent("DELETE", `/api/v1/me/tokens/${encodeURIComponent(id)}`);
}

export async function fetchWebhooks(): Promise<Webhook[]> {
  return getList<Webhook>("/api/v1/me/webhooks", "webhooks");
}

export async function createWebhook(input: { url: string; events: string[] }): Promise<CreatedWebhook> {
  return mutation<CreatedWebhook>("POST", "/api/v1/me/webhooks", input);
}

export async function testWebhook(id: string): Promise<void> {
  const response = await fetch(`/api/v1/me/webhooks/${encodeURIComponent(id)}/test`, { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw await responseError(response, "webhook test failed");
}

export async function deleteWebhook(id: string): Promise<void> {
  return noContent("DELETE", `/api/v1/me/webhooks/${encodeURIComponent(id)}`);
}

export function exportURL() {
  return "/api/v1/me/export";
}

async function getList<T>(path: string, key: string): Promise<T[]> {
  const response = await fetch(path, { credentials: "same-origin" });
  if (!response.ok) throw await responseError(response, "request failed");
  const payload = (await response.json()) as Record<string, T[]>;
  return payload[key] ?? [];
}

async function mutation<T>(method: string, path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw await responseError(response, "request failed");
  return response.json() as Promise<T>;
}

async function noContent(method: string, path: string): Promise<void> {
  const response = await fetch(path, { method, credentials: "same-origin" });
  if (!response.ok) throw await responseError(response, "request failed");
}

async function responseError(response: Response, fallback: string): Promise<Error> {
  try {
    const payload = (await response.json()) as { error?: string };
    return new Error(payload.error || fallback);
  } catch {
    return new Error(fallback);
  }
}
