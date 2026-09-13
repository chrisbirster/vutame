# M2 — Linktree parity and profile design

Status: **in progress**

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
- [ ] CI green on the exact media-upload feature head.
- [ ] Merge the media-upload feature into `dev`.

## Slice 3 — richer link presentation

- [x] Add optional link thumbnails and render them through the shared public/preview surface.
- [ ] Add featured/pinned links.
- [ ] Add scheduled links and temporary visibility windows.
- [ ] Add safe generic OpenGraph preview metadata.
- [ ] Add explicit rich integrations for high-value providers where embedding is safe and useful.
- [ ] Ensure rich cards degrade to ordinary links when metadata fetches fail.

## Slice 4 — sharing, SEO, accessibility, and import

- [ ] Generate downloadable/shareable profile QR codes.
- [ ] Generate profile-specific document title, description, canonical URL, and OpenGraph metadata.
- [ ] Add keyboard, contrast, reduced-motion, and screen-reader passes across every theme.
- [ ] Add mobile performance budgets and image-loading rules.
- [ ] Add a simple importer for conventional link-in-bio link lists where technically and legally practical.
- [ ] Verify profiles remain usable without JavaScript after server/meta improvements planned for this slice.

## M2 exit criteria

A creator can build a polished Vuta with a distinct visual identity, avatar/media, recognizable social links, rich/featured/scheduled content, sharing/SEO support, and strong mobile accessibility. The creator preview must use the same presentation primitives as the public profile so saved output cannot materially differ from what was previewed.
