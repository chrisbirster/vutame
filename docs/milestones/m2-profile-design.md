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
- [ ] CI green on the exact feature head.
- [ ] Merge the slice into `dev`.

## Slice 2 — media and typed social links

- [ ] Add an avatar/media upload abstraction with validation, size limits, and safe content types.
- [ ] Add durable uploaded-avatar metadata and replacement/delete behavior.
- [ ] Define typed social link kinds for major creator/developer platforms.
- [ ] Render recognizable social icons without making arbitrary user HTML possible.
- [ ] Add editor controls for choosing link kind and platform-specific metadata.
- [ ] Keep generic website/project links as a first-class fallback.
- [ ] Add accessibility labels for icon-only or platform-enhanced links.

## Slice 3 — richer link presentation

- [ ] Add optional link thumbnails.
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
