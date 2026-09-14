export type AnalyticsSummary = {
  profile_views: number;
  link_clicks: number;
  unique_visitors: number;
};

export type AnalyticsDailyPoint = {
  date: string;
  profile_views: number;
  link_clicks: number;
  unique_visitors: number;
};

export type AnalyticsLinkMetric = {
  link_id: string;
  label: string;
  clicks: number;
};

export type AnalyticsBreakdown = {
  name: string;
  count: number;
};

export type AnalyticsDashboard = {
  days: number;
  start_date: string;
  end_date: string;
  summary: AnalyticsSummary;
  series: AnalyticsDailyPoint[];
  top_links: AnalyticsLinkMetric[];
  referrers: AnalyticsBreakdown[];
  devices: AnalyticsBreakdown[];
};

export class AnalyticsAPIError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "AnalyticsAPIError";
    this.status = status;
  }
}

export async function fetchAnalyticsDashboard(days = 30): Promise<AnalyticsDashboard> {
  const response = await fetch(`/api/v1/me/analytics?days=${encodeURIComponent(String(days))}`, {
    credentials: "same-origin",
  });
  if (!response.ok) {
    let message = "analytics request failed";
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) message = payload.error;
    } catch {
      // Preserve the generic message when the response is not JSON.
    }
    throw new AnalyticsAPIError(message, response.status);
  }
  return response.json() as Promise<AnalyticsDashboard>;
}
