# InstantAppoint

Appointment scheduling and WhatsApp reminder tool for small service businesses. Manage your calendar and send appointment reminders directly to clients via WhatsApp — no third-party messaging service required.

## What it does

- **Calendar** — create and manage appointments with client name, phone number, time, location, and notes
- **WhatsApp** — link your WhatsApp account by scanning a QR code; the app uses it to send reminders to clients
- **Reminders** — (coming soon) automated reminder messages sent before appointments

Built for solo operators and small businesses where WhatsApp is the primary way to communicate with clients.

## Go server (migration in progress)

The app is being migrated from Next.js + a PocketBase server + a Node sidecar to a **single Go
binary** that embeds PocketBase as a library and serves the React SPA out of `//go:embed`. The
sections below the migration notice still describe the old stack until the port lands.

| | |
|---|---|
| PocketBase | **v0.40.6**, pinned in `go.mod` (embedded as a library, not run as a separate server) |
| Go | **1.27 or newer** — required by PocketBase v0.40.6 |
| SQLite driver | `modernc.org/sqlite`, pure Go — the binary builds with `CGO_ENABLED=0` and needs no C compiler |

### Go 1.27 is a hard requirement

PocketBase v0.40.6 declares `go 1.27`, and so does our `go.mod`. On a machine with an older Go
the build still works only because the default `GOTOOLCHAIN=auto` downloads Go 1.27 from the
module proxy on demand. `GOTOOLCHAIN=local` with Go 1.26 fails outright:

```
go: go.mod requires go >= 1.27 (running go 1.26.0; GOTOOLCHAIN=local)
```

CI images and Dockerfiles must therefore use `golang:1.27` or newer — a pinned older image only
builds if it can reach `proxy.golang.org`, and an air-gapped or `GOTOOLCHAIN=local` build breaks.

### Build

```bash
cd web && npm install && npm run build   # produces web/dist, which is embedded
cd ..
CGO_ENABLED=0 go build -o instantappoint ./cmd/server
```

The UI is compiled into the binary: `instantappoint` can be copied anywhere on its own and still
serves the app. Changing anything under `web/` needs a `npm run build` **and** a `go build`.

`web/dist` is gitignored apart from a committed `.gitkeep`, because `//go:embed all:dist` is a
compile error when the directory is missing — a fresh clone must be able to `go build` before
Vite has ever run. A binary built that way serves a placeholder page explaining how to build the
UI; the admin UI is unaffected. (The `all:` prefix is required: Vite emits `_`-prefixed paths
that a plain `//go:embed` skips.)

### Run

```bash
./instantappoint serve                                  # http://127.0.0.1:8090
./instantappoint superuser upsert you@example.com <pw>   # admin UI account, see below
./instantappoint seed                                    # dev fixtures, see below
./instantappoint migrate up                              # apply migrations without serving
```

`serve` applies pending migrations on boot. Schema lives in `migrations/` as Go code;
`Automigrate` is off, so edits made in the admin UI are never written back as migration files.

Data lives in `pb_data/` next to the binary (`--dir` to move it). It is gitignored; never commit it.

### Administrator account

`superuser upsert` takes the email and password as arguments and never prompts, so it works in
a Dockerfile, an entrypoint script or CI with stdin closed. It creates the account if the email
is unknown and resets the password if it is not, which makes it safe to run on every boot.

```bash
./instantappoint superuser upsert you@example.com <at-least-10-characters>
```

Then sign in at `/_/`. (`superuser create` exists too, but it fails on an email that already
exists — prefer `upsert`.)

### The seed command

Registration sends a verification email and the login page refuses an unverified account, so
a fresh database cannot be logged into until mail delivery is configured. `seed` writes an
already-verified test account straight to the collection:

```bash
./instantappoint seed
SEED_EMAIL=dev@instantappoint.test
SEED_PASSWORD=devpassword123
SEED_USER_ID=giecjjw8596s8wu
```

One `KEY=value` per line on stdout (the prose goes to stderr), so a script can read it with
`eval "$(./instantappoint seed 2>/dev/null | grep '^SEED_')"`. It applies migrations first, so
it works against a database that has never been served, and it is idempotent: re-running
restores the password and the verified flag rather than failing. **Development only** — the
credentials are public, in this file and in the source.

| Path | Serves |
|---|---|
| `/` and any other extensionless path | the SPA shell, so client-side deep links survive a refresh |
| `/assets/...` | the embedded build output; a missing file is a real 404, never the HTML shell |
| `/api/...` | PocketBase's REST API; an unknown API path is a JSON 404, never the HTML shell |
| `/_/` | the PocketBase admin UI |

### Dev loop

Two terminals:

```bash
go run ./cmd/server serve      # :8090 — API, admin UI, migrations
```

```bash
cd web && npm run dev          # :5173 — SPA with HMR, proxies /api and /_/ to :8090
```

Work against **http://localhost:5173**. The Go server does not need rebuilding for frontend
changes in this mode, because Vite serves `web/src` directly rather than the embedded bundle.

### Tests

```bash
go test ./...                  # Go
cd web && npm test             # SPA (vitest + jsdom)
cd web && npm run build        # type-check + production build
```

## Tech stack

- **Next.js** — frontend and API routes
- **PocketBase** — database and auth
- **whatsapp-web.js** — WhatsApp automation via Puppeteer, running as a sidecar service

## Quickstart

```bash
./dev.sh
```

That's it. The script starts all three services:

| Service | Port |
|---|---|
| Next.js app | :3000 |
| PocketBase | :8090 |
| WhatsApp service | :3003 |

Press `Ctrl+C` to stop everything.

## Manual startup

If you prefer to run services individually, open three terminals:

```bash
# PocketBase
pocketbase serve --dir=pb_data

# WhatsApp service
cd whatsapp-service && npm start

# Next.js
npm run dev
```

## Tests

```bash
npm test
```

## WhatsApp Cloud API webhooks

Meta does not call this app directly. All WhatsApp webhooks go through the Neurocrow webhook
router (`hooks.neurocrow.com`), which answers Meta's verification, checks Meta's signature, and
POSTs each event to `/api/webhook` signed with a shared secret:

- `X-Neurocrow-Timestamp`: unix seconds
- `X-Neurocrow-Signature-256`: `sha256=` + hex HMAC-SHA256(secret, `timestamp + "." + body`)
- `X-Neurocrow-Delivery-Id`: unique per delivery

`/api/webhook` rejects (401) any POST without a valid signature or with a timestamp more than
5 minutes off. There is no GET handler any more. Delivery is at-least-once, so messages are
deduped on the WhatsApp message id. A message more than 15 minutes old (by its own timestamp) is
stored but gets no AI auto-reply.

| Variable | Required | Description |
|---|---|---|
| `NEUROCROW_ROUTER_SECRET` | yes | Shared secret for router deliveries. Must equal the router's `DEST_SECRET_INSTANTAPPOINT`. If unset, every webhook POST is rejected. |

## WhatsApp setup

1. Open the app and go to the **WhatsApp Setup** tab
2. Click **Connect WhatsApp**
3. Scan the QR code with your phone (WhatsApp → Settings → Linked Devices → Link a Device)
4. Done — your session is saved and will auto-reconnect on restart

## Phone number format

Numbers must include the country code, digits only:
- US: `15512345678`
- Mexico: `5215512345678` (use `521` prefix for Mexican mobile numbers, not `52`)
