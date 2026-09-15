export type BillingSubscription = {
  id: string;
  provider_subscription_id: string;
  customer_id: string;
  price_id: string;
  plan: string;
  status: string;
  current_period_end?: string;
  cancel_at_period_end: boolean;
  updated_at: string;
};

export type BillingState = {
  plan: "free" | "pro";
  subscription?: BillingSubscription;
  entitlements: string[];
  billing_configured: boolean;
};

export async function fetchBillingState(): Promise<BillingState> {
  return requestJSON<BillingState>("/api/v1/me/billing", { credentials: "same-origin" });
}

export async function startCheckout(): Promise<string> {
  const response = await requestJSON<{ url: string }>("/api/v1/me/billing/checkout", {
    method: "POST",
    credentials: "same-origin",
  });
  return response.url;
}

export async function openBillingPortal(): Promise<string> {
  const response = await requestJSON<{ url: string }>("/api/v1/me/billing/portal", {
    method: "POST",
    credentials: "same-origin",
  });
  return response.url;
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  if (!response.ok) {
    let message = "billing request failed";
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) message = payload.error;
    } catch {
      // Keep stable fallback.
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}
