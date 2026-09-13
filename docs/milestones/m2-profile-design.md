# M2 — Linktree parity and profile design

Status: **complete**

Goal: make Vutame polished and expressive enough to replace a conventional link-in-bio product while keeping the public profile fast, accessible, and consistent with the creator editor.

## Slice 1 — themes and live preview

- [x] Add a persisted profile theme field with `midnight` as the backward-compatible default.
- [x] Validate theme IDs in the Go domain layer rather than trusting client values.
- [x] Ship four initial tokenized presets: Midnight, Paper, Neon, and Forest.
- [x] Use one `ProfileSurface` component for both the public profile and editor preview.
- [x] Render real avatar URLs with a generated initial fallback.
- [x] Add a responsive live mobile preview to the creator dashboard.
- [x] Preview display name, bio, avatar URL, and theme changes before save.
- [x] Persist the selected theme through SQLite/Turso and expose it in public profile JSON.
- [x] Add domain, SQLite, HTTP lifecycle, and Turso-engine theme coverage.
- [x] CI green on the exact feature head.
- [x] Merge the slice into `dev`.

## Slice 2 — media and typed social links

- [x] Add a vendor-neutral avatar/media blob-store abstraction.
- [x] Add a filesystem blob adapter for local and single-replica/shared-volume deployments.
- [x] Enforce a 5 MB image limit and byte-sniff JPEG, PNG, WebP, and GIF instead of trusting client MIME metadata.
- [x] Add durable `media_assets` metadata with one managed avatar slot per user.
- [x] Add transactional avatar replacement/delete behavior and immutable `/media/<id>` URLs.
- [x] Add authenticated same-origin avatar upload/delete endpoints and immutable public media serving.
- [x] Add managed-avatar controls to the creator editor while retaining external avatar URLs as a fallback.
- [x] Define typed link kinds for website, project, GitHub, YouTube, Instagram, TikTok, X, Bluesky, LinkedIn, Spotify, newsletter, and shop links.
- [x] Render recognizable platform badges without allowing arbitrary user HTML.
- [x] Add editor controls for choosing link kind and optional rich-card thumbnail metadata.
- [x] Keep generic website/project links as first-class fallbacks.
- [x] Keep visible labels and kind text alongside platform badges rather than relying on icon-only controls.
- [x] Validate link kinds and thumbnail URLs in the Go domain layer.
- [x] Persist typed-link metadata through the shared SQLite/Turso store.
- [x] Add domain, SQLite, authenticated HTTP, and Turso-engine coverage for rich links.
- [x] CI green on the exact rich-links feature head.
- [x] Merge the rich-links feature into `dev`.
- [x] Add filesystem/service, authenticated HTTP, and Turso-engine coverage for managed avatars.
- [x] CI green on the exact media-upload feature head.
- [x] Merge the media-upload feature into `dev`.

## Slice 3 — richer link presentation

- [x] Add optional link thumbnails and render them through the shared public/preview surface.
- [x] Add featured/pinned links that sort ahead of ordinary links while preserving manual order within each group.
- [x] Add scheduled links with optional RFC3339 start/end visibility windows.
- [x] Keep scheduled/future/expired links in the owner dashboard while filtering them from public/discovery responses using server time.
- [x] Add local datetime editor controls that convert to UTC instants before mutation requests.
- [x] Reuse schedule/featured behavior in the shared live-preview/public profile renderer.
- [x] Add fixed-clock domain, SQLite, authenticated HTTP, and Turso-engine coverage for feature/schedule behavior.
- [x] CI green on the exact featured/scheduled feature head.
- [x] Merge the featured/scheduled feature into `dev`.
- [x] Add safe generic OpenGraph preview metadata behind an authenticated HTTPS-only fetcher with redirect/body/time limits and private-network protections.
- [x] Add provider-aware enrichment for GitHub, YouTube, Instagram, TikTok, X, Bluesky, LinkedIn, and Spotify while keeping Vutame cards as ordinary links instead of loading third-party iframe code.
- [x] Ensure preview-enrichment failures degrade to ordinary typed links and never block import or rendering.

## Slice 4 — sharing, SEO, accessibility, and import

- [x] Generate downloadable/shareable profile QR codes from the canonical `vuta.me/@handle` URL.
- [x] Generate profile-specific document title, description, canonical URL, OpenGraph metadata, and Twitter-card metadata on the server.
- [x] Add keyboard focus defaults, reduced-motion handling, semantic/label improvements, and decorative-image treatment across the shared profile themes.
- [x] Add image dimensions, lazy loading/async decoding, and a CI-enforced gzip budget for shipped JavaScript/CSS.
- [x] Add a creator import tool for CSV, tab/pipe-separated, and copied link lists with a 50-link batch cap and review step.
- [x] Render existing public profiles as escaped semantic `<noscript>` HTML so names, bios, and public links remain usable without JavaScript.
- [x] Add unit/HTTP coverage for profile metadata/no-JS rendering, QR generation, and safe preview metadata.
- [x] CI green on the exact M2 finish feature head.
- [x] Merge the M2 finish feature into `dev`.

## M2 exit criteria

**Satisfied.** A creator can build a polished Vuta with a distinct visual identity, managed avatar/media, recognizable typed social links, thumbnails, featured/scheduled content, safe metadata enrichment, QR sharing, profile-specific SEO, import tools, and strong mobile accessibility/performance defaults. The creator preview uses the same presentation primitives as the public profile so saved output cannot materially differ from what was previewed.
