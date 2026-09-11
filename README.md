# Vutame

Vutame is a social profile and link-sharing platform.

- **`vutame.com`** — marketing, discovery, onboarding, account management, and the network.
- **`vuta.me/@handle`** — the short public profile people share.

The first product goal is Linktree-level utility. The longer-term differentiator is a creator discovery graph and an optional AT Protocol layer where public profile/link records can become portable.

## Stack

- Go 1.26 `net/http`
- Go `embed` for the production SPA
- Vite
- Solid 2
- Solid Router
- StyleX
- TypeScript

## Branches

Development uses:

```text
feature/* -> dev -> main -> vX.Y.Z
```

Feature pull requests target `dev`. `main` is release-only. See [`docs/branching-and-releases.md`](docs/branching-and-releases.md).

## Develop

Run the API:

```bash
npm run dev:api
```

Run Vite in another terminal:

```bash
npm run dev
```

Vite proxies `/api` to `http://127.0.0.1:8080`.

Open:

- `http://localhost:5173/`
- `http://localhost:5173/discover`
- `http://localhost:5173/@chrisdontmiss`

## Build

```bash
npm install
npm run build
./vutame
```

`vite build` writes to `internal/web/dist`. The Go build embeds that directory so the deployable artifact is one binary.

## Verify

```bash
npm run verify
```

## Documentation

Start with [`docs/README.md`](docs/README.md), then read the [architecture](docs/architecture.md), [roadmap](docs/roadmap.md), and current [M0 milestone](docs/milestones/m0-foundation.md).
