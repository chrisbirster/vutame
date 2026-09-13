export const LINK_KINDS = [
  { id: "website", label: "Website", badge: "WEB" },
  { id: "project", label: "Project", badge: "PRJ" },
  { id: "github", label: "GitHub", badge: "GH" },
  { id: "youtube", label: "YouTube", badge: "YT" },
  { id: "instagram", label: "Instagram", badge: "IG" },
  { id: "tiktok", label: "TikTok", badge: "TT" },
  { id: "x", label: "X", badge: "X" },
  { id: "bluesky", label: "Bluesky", badge: "BS" },
  { id: "linkedin", label: "LinkedIn", badge: "IN" },
  { id: "spotify", label: "Spotify", badge: "SP" },
  { id: "newsletter", label: "Newsletter", badge: "MAIL" },
  { id: "shop", label: "Shop", badge: "SHOP" },
  { id: "social", label: "Social", badge: "SOC" },
] as const;

export type LinkKind = (typeof LINK_KINDS)[number]["id"];

export function normalizeLinkKind(kind: string | undefined): LinkKind {
  return LINK_KINDS.some((candidate) => candidate.id === kind)
    ? (kind as LinkKind)
    : "website";
}

export function linkKindMeta(kind: string | undefined) {
  const normalized = normalizeLinkKind(kind);
  return LINK_KINDS.find((candidate) => candidate.id === normalized) ?? LINK_KINDS[0];
}
