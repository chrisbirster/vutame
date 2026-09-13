# Deployment

Vutame ships as one Go binary/container with the Solid SPA embedded into it.

Hosted deployments run with `VUTAME_ENV=production`. In that mode startup **requires persistent storage**; the seeded in-memory profile store is development-only.

## Production database

Vutame uses the current Turso Database Go driver (`turso.tech/database/tursogo`) in sync mode:

```text
Go service
   │
   ├─ database/sql
   │    └─ local Turso replica
   │
   └─ periodic push/pull
        └─ Turso Cloud
```

The local replica gives normal `database/sql` semantics to the existing auth/profile stores. Turso Cloud is the durable remote copy. The service bootstraps an empty local replica from the remote database, pushes/pulls on a bounded interval, and performs a final push during graceful shutdown.

Required runtime variables:

```bash
VUTAME_ENV=production
VUTAME_TURSO_REMOTE_URL='https://<database>.<region>.turso.io'
VUTAME_TURSO_AUTH_TOKEN='<database-token>'
VUTAME_TURSO_LOCAL_PATH='/tmp/vutame.db'
VUTAME_TURSO_SYNC_INTERVAL='5s'
```

`VUTAME_TURSO_LOCAL_PATH` may point at ephemeral storage because the cloud database is authoritative, but a small persistent volume reduces recovery work and narrows the window between a local write and the next cloud push.

Do not configure `VUTAME_DATABASE_DSN` at the same time as Turso sync settings. `VUTAME_DATABASE_DSN` remains the local-development SQLite path.

## Schema management

Normal application startup never creates or migrates tables. Atlas owns the desired schema in `internal/dbschema/schema.sql`.

Create/provision the Turso database and token, then give Atlas a libSQL connection URL:

```bash
export VUTAME_ATLAS_URL='libsql://<database>.<region>.turso.io?authToken=<database-token>'

npm run db:schema:production:plan
npm run db:schema:production:apply
```

Apply the schema **before** starting a new Vutame environment. The application verifies the required tables and exits instead of serving against an unknown schema.

## Authentication secrets

Persistent deployments require a stable HMAC/session secret:

```bash
VUTAME_AUTH_SECRET='<at-least-32-random-bytes>'
VUTAME_COOKIE_SECURE=1
```

Production email delivery uses SMTP with mandatory STARTTLS:

```bash
VUTAME_SMTP_ADDR='smtp.example.com:587'
VUTAME_SMTP_USERNAME='<smtp-user>'
VUTAME_SMTP_PASSWORD='<smtp-password>'
VUTAME_AUTH_EMAIL_FROM='Vutame <login@vutame.com>'
```

An Amazon SES SMTP endpoint and SES SMTP credentials can be supplied through the same variables. `VUTAME_AUTH_LOG_CODES=1` is local-development-only and is rejected when secure cookies are enabled.

## Domains

Set the canonical public origins explicitly in hosted environments:

```bash
VUTAME_MARKETING_ORIGIN='https://vutame.com'
VUTAME_PROFILE_ORIGIN='https://vuta.me'
```

Both domains may terminate at the same service initially. `vutame.com` is the application/network surface; `vuta.me/@handle` is the public identity surface.

## Container contract

The Docker image sets `VUTAME_ENV=production` itself. Therefore a container started without database settings fails fast rather than quietly serving temporary seeded data.

The release build is intentionally no-CGO:

```bash
CGO_ENABLED=0 go build ./cmd/vutame
```

The Turso Go driver uses prebuilt platform libraries through `purego`, so the application does not need a C toolchain at build time.

## Deployment order

1. Provision the Turso database and access token.
2. Run the Atlas production plan and inspect it.
3. Apply the Atlas schema.
4. Configure Vutame database/auth/SMTP/domain secrets.
5. Deploy the container.
6. Verify `/api/v1/healthz` and sign-in.
7. Claim a test handle, edit it, restart the service, and confirm the same profile remains available.

Turso Database is pre-1.0. Keep provider backups/recovery enabled and test restoration before public beta.
