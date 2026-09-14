export type ContactBlock = {
  enabled: boolean;
  heading: string;
  description: string;
  consent_text: string;
  button_label: string;
};

export type ContactSubmission = {
  id: string;
  email: string;
  campaign?: string;
  created_at: string;
};

export type DataSettings = {
  analytics_retention_days: number;
  contact_retention_days: number;
};

export async function fetchPublicContactBlock(handle: string): Promise<ContactBlock | null> {
  const response = await fetch(`/api/v1/profiles/${encodeURIComponent(handle)}/contact-block`);
  if (response.status === 404 || response.status === 503) return null;
  if (!response.ok) throw await growthError(response, "contact block request failed");
  return response.json() as Promise<ContactBlock>;
}

export async function submitPublicContact(handle: string, input: {
  email: string;
  consent: boolean;
  campaign?: string;
  website?: string;
}): Promise<void> {
  const response = await fetch(`/api/v1/profiles/${encodeURIComponent(handle)}/contacts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await growthError(response, "contact submission failed");
}

export async function fetchOwnedContactBlock(): Promise<ContactBlock> {
  return ownedJSON<ContactBlock>("/api/v1/me/contact-block");
}

export async function updateOwnedContactBlock(input: ContactBlock): Promise<ContactBlock> {
  return ownedMutation<ContactBlock>("/api/v1/me/contact-block", input);
}

export async function fetchContacts(limit = 100): Promise<ContactSubmission[]> {
  const response = await fetch(`/api/v1/me/contacts?limit=${encodeURIComponent(String(limit))}`, { credentials: "same-origin" });
  if (!response.ok) throw await growthError(response, "contacts request failed");
  const payload = (await response.json()) as { contacts: ContactSubmission[] };
  return payload.contacts;
}

export async function fetchDataSettings(): Promise<DataSettings> {
  return ownedJSON<DataSettings>("/api/v1/me/data-retention");
}

export async function updateDataSettings(input: DataSettings): Promise<DataSettings> {
  return ownedMutation<DataSettings>("/api/v1/me/data-retention", input);
}

async function ownedJSON<T>(path: string): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin" });
  if (!response.ok) throw await growthError(response, "growth request failed");
  return response.json() as Promise<T>;
}

async function ownedMutation<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw await growthError(response, "growth update failed");
  return response.json() as Promise<T>;
}

async function growthError(response: Response, fallback: string): Promise<Error> {
  try {
    const payload = (await response.json()) as { error?: string };
    return new Error(payload.error || fallback);
  } catch {
    return new Error(fallback);
  }
}
