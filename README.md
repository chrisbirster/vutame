# Vutame

Vutame is a social profile and link-sharing platform.

- **`vutame.com`** — marketing, discovery, onboarding, and the network.
- **`vuta.me/@handle`** — the short public profile people share.

The first product slice is intentionally Linktree-simple, but the architecture leaves room for an AT Protocol-native social layer where public profiles and links can become portable records.

## Stack

- Go 1.26 `net/http`
- Go `embed` for the production SPA
- Vite
- Solid 2
- Solid Router
- StyleX
- TypeScript

The project shape follows the same embedded-SPA pattern used by the other Go/Solid repositories.

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

See [`docs/architecture.md`](docs/architecture.md) for the product split and ATProto direction.
