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
- SQLite locally with Atlas-managed schema
- Turso Database sync for hosted persistence

## Branches

Development uses:

```text
feature/* -> dev -> main -> vX.Y.Z
```

Feature pull requests target `dev`. `main` is release-only. See [`docs/branching-and-releases.md`](docs/branching-and-releases.md).

## Develop

Install dependencies and apply the local Atlas schema:

```bash
npm install
npm run db:schema:apply
```

Run the API with durable local profiles and development-only email-code logging:

```bash
VUTAME_DATABASE_DSN='file:vutame.db' \
VUTAME_AUTH_SECRET='replace-with-at-least-32-random-bytes' \
VUTAME_AUTH_LOG_CODES=1 \
VUTAME_COOKIE_SECURE=0 \
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
- `http://localhost:5173/signin`
- `http://localhost:5173/create`
- `http://localhost:5173/settings`
- `http://localhost:5173/@chrisdontmiss`

The no-database startup path serves the seeded public demo only in development. Authenticated editing requires managed durable storage, and `VUTAME_ENV=production` refuses to start without a persistent backend.

## Hosted database

Hosted Vutame uses a local Turso replica synchronized with Turso Cloud:

```bash
VUTAME_ENV=production
VUTAME_TURSO_REMOTE_URL='https://database.region.turso.io'
VUTAME_TURSO_AUTH_TOKEN='database-token'
VUTAME_TURSO_LOCAL_PATH='/tmp/vutame.db'
VUTAME_TURSO_SYNC_INTERVAL='5s'
```

Atlas applies `internal/dbschema/schema.sql` to the remote database before deployment. See [`docs/deployment.md`](docs/deployment.md) for the full setup and rollout order.

## Hosted auth email

Production-style email delivery is provider-neutral SMTP with mandatory STARTTLS. Configure:

```bash
VUTAME_SMTP_ADDR='smtp.example.com:587'
VUTAME_SMTP_USERNAME='smtp-user'
VUTAME_SMTP_PASSWORD='smtp-password'
VUTAME_AUTH_EMAIL_FROM='Vutame <login@vutame.com>'
```

These settings can be backed by an SMTP provider such as Amazon SES SMTP credentials. Do not enable `VUTAME_AUTH_LOG_CODES` in hosted environments; the server rejects development code logging while secure cookies are enabled.

## Build

```bash
npm install
npm run build
./vutame
```

`vite build` writes to `internal/web/dist`. The Go build embeds that directory so the deployable artifact is one binary. Server tests and builds run with `CGO_ENABLED=0`.

## Verify

```bash
npm run verify
```

## Documentation

Start with [`docs/README.md`](docs/README.md), then read the [architecture](docs/architecture.md), [deployment guide](docs/deployment.md), [roadmap](docs/roadmap.md), and current [M1 milestone](docs/milestones/m1-accounts-persistence.md).
