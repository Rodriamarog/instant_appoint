# InstantAppoint

Appointment scheduling with WhatsApp reminders and an AI-answered WhatsApp inbox,
for small service businesses.

Create appointments in a calendar, let the app send each client a reminder over
WhatsApp before their slot, and read and answer the replies in the app. A message
that arrives with nobody watching gets an automatic reply from Gemini, which acts
as a booking assistant for the business.

## The whole thing is one binary

`instantappoint` is a single executable. It embeds
[PocketBase](https://pocketbase.io) v0.40.6 as a Go library — database, auth,
REST API, admin UI, realtime and cron — and serves the compiled React SPA out of
`//go:embed`. There is no separate database server, no Node process and no
frontend server. Copy the binary to a machine and run it.

| | |
|---|---|
| Go | **1.27 or newer** — required by PocketBase v0.40.6 |
| PocketBase | **v0.40.6**, pinned in `go.mod`, embedded as a library |
| SQLite driver | `modernc.org/sqlite`, pure Go — builds with `CGO_ENABLED=0`, no C compiler |
| Frontend | Vite 7 + React 19 + TypeScript + React Router 7 + Tailwind v4, in `web/` |
| Messaging | Meta WhatsApp Cloud API, server-side |
| AI replies | Google Gemini |

```
cmd/server/        wiring: pocketbase.New(), the commands, one OnServe hook
internal/          all behaviour, one package per concern
migrations/        the schema, as Go code — the only source of schema truth
web/               the SPA; web/dist is embedded into the binary
pb_migrations/     the retired JavaScript migrations, kept for history only.
                   Nothing runs them.
whatsapp-service/  the retired QR sidecar. Dormant and unwired; see its README.
```

### Go 1.27 is a hard requirement

PocketBase v0.40.6 declares `go 1.27`, and so does our `go.mod`. On a machine
with an older Go the build still works only because the default
`GOTOOLCHAIN=auto` downloads Go 1.27 from the module proxy on demand.
`GOTOOLCHAIN=local` with Go 1.26 fails outright:

```
go: go.mod requires go >= 1.27 (running go 1.26.0; GOTOOLCHAIN=local)
```

CI images and Dockerfiles must therefore use `golang:1.27` or newer — a pinned
older image only builds if it can reach `proxy.golang.org`, and an air-gapped or
`GOTOOLCHAIN=local` build breaks.

## Build

From a fresh clone, with Node 20+ and Go 1.27+:

```bash
cd web && npm install && npm run build   # produces web/dist, which is embedded
cd ..
CGO_ENABLED=0 go build -o instantappoint ./cmd/server
```

That is the whole build. The UI is compiled into the binary, so `instantappoint`
can be copied anywhere on its own and still serve the app. Changing anything
under `web/` needs a `npm run build` **and** a `go build`.

`web/dist` is gitignored apart from a committed `.gitkeep`, because
`//go:embed all:dist` is a compile error when the directory is missing — a fresh
clone must be able to `go build` before Vite has ever run. A binary built that
way serves a placeholder page explaining how to build the UI; the admin UI and
the API are unaffected. (The `all:` prefix is required: Vite emits `_`-prefixed
paths that a plain `//go:embed` skips.)

## Run

```bash
./instantappoint serve                                   # http://127.0.0.1:8090
./instantappoint superuser upsert you@example.com <pw>    # admin UI account
./instantappoint seed                                     # dev fixtures
./instantappoint migrate up                               # migrate without serving
```

`serve` applies pending migrations on boot and starts the reminder cron. The
schema lives in `migrations/` as Go code; `Automigrate` is off, so edits made in
the admin UI are never written back as migration files.

Data lives in `pb_data/` next to the binary (`--dir` to move it). It is
gitignored; never commit it.

| Path | Serves |
|---|---|
| `/` and any other extensionless path | the SPA shell, so client-side deep links survive a refresh |
| `/assets/...` | the embedded build output; a missing file is a real 404, never the HTML shell |
| `/api/...` | PocketBase's REST API plus the handlers below; an unknown API path is a JSON 404, never the HTML shell |
| `/_/` | the PocketBase admin UI |

The SPA reads and writes PocketBase's record API directly. Only the things that
spend a secret are handlers of our own:

| Route | What it does |
|---|---|
| `POST /api/whatsapp/exchange-token` | completes Meta Embedded Signup; spends the app secret |
| `GET /api/whatsapp/templates` | lists the account's approved message templates |
| `POST /api/whatsapp/templates/create` | submits a new template to Meta |
| `POST /api/whatsapp/templates/send` | sends a template as a test |
| `POST /api/chat/send` | sends a human reply, from the number the thread arrived on |
| `POST /api/webhook` (and `/webhook`) | inbound deliveries from the Neurocrow router |

## Environment variables

**Four, all required in production.** Each is read at boot; a missing one logs a
warning naming what will not work rather than refusing to start, so the rest of
the app keeps running. Those warnings go to PocketBase's own log — the `_logs`
collection, readable under **Logs** in the admin UI — not to stdout, so a clean
terminal is not evidence that all four are set.

| Variable | What breaks without it |
|---|---|
| `GEMINI_API_KEY` | Inbound messages are still stored, but no automatic reply is written or sent. |
| `META_APP_ID` | Connecting a WhatsApp number fails. Numbers already connected keep working. |
| `META_APP_SECRET` | Same as above — the Embedded Signup token exchange needs both. |
| `NEUROCROW_ROUTER_SECRET` | **Every webhook delivery is rejected.** No customer message arrives at all. Must equal the router's `DEST_SECRET_INSTANTAPPOINT`. |

Set them in the process environment, or in a systemd `EnvironmentFile`. Nothing
reads a `.env` file, and `.env*` is gitignored.

```bash
GEMINI_API_KEY=... \
META_APP_ID=... \
META_APP_SECRET=... \
NEUROCROW_ROUTER_SECRET=... \
./instantappoint serve
```

### Two test-only seams, which are not requirements

Neither of these belongs in a deployment. They exist so an automated check can
drive the Meta and Gemini paths end to end without a live business account and a
real API key, by pointing the host at a stand-in. **Both log a warning when set**
— again in the admin UI's log — because an environment that sets one by accident
is talking to something that is not Meta, or not Google.

| Variable | Default | Effect when set |
|---|---|---|
| `META_GRAPH_BASE_URL` | Meta's Graph host | Every Graph call goes to this host instead. |
| `GEMINI_BASE_URL` | Google's Generative Language host | Replies are written by whatever answers here. |

If either warning shows up in a production log, treat it as an incident.

## First boot

1. `./instantappoint superuser upsert you@example.com <at-least-10-characters>`,
   then sign in at `/_/`. It takes the email and password as arguments and never
   prompts, so it works in a Dockerfile, an entrypoint or CI with stdin closed.
   It creates the account if the email is unknown and resets the password if it
   is not, which makes it safe to run on every boot. (`superuser create` exists
   too, but fails on an email that already exists — prefer `upsert`.)
2. **Configure SMTP in the admin UI.** Mail settings live in `pb_data`, not in
   code and not in an environment variable. Registration sends a verification
   email and the login page refuses an unverified account, so without SMTP nobody
   can sign up. Note that the verification request reports success either way —
   it cannot be used to tell whether mail was actually delivered.
3. Register a business account at `/register`, then connect a WhatsApp number on
   the **WhatsApp** page via Meta Embedded Signup.
4. Point the Neurocrow router's destination for this app at
   `https://<your-host>/api/webhook`, with `DEST_SECRET_INSTANTAPPOINT` equal to
   `NEUROCROW_ROUTER_SECRET`.

### The seed command, for development

A fresh database cannot be logged into until SMTP is configured. `seed` writes an
already-verified test account, plus a fake connected WhatsApp account, straight
to the collections:

```bash
./instantappoint seed
SEED_EMAIL=dev@instantappoint.test
SEED_PASSWORD=devpassword123
SEED_USER_ID=giecjjw8596s8wu
SEED_WHATSAPP_ACCOUNT_ID=...
SEED_WHATSAPP_PHONE_NUMBER_ID=...
SEED_WHATSAPP_WABA_ID=...
```

One `KEY=value` per line on stdout (prose goes to stderr), so a script can read
it with `eval "$(./instantappoint seed 2>/dev/null | grep '^SEED_')"`. It applies
migrations first, so it works against a database that has never been served, and
it is idempotent: re-running restores the password and the verified flag rather
than failing. **Development only** — the credentials are public, in this file and
in the source.

Build the binary somewhere other than your temp directory. PocketBase treats an
executable under `os.TempDir()` as `go run` and switches on dev mode, which dumps
every SQL statement to stdout and contaminates `seed`'s machine-readable output.
Or pass `--dev=false`.

## Dev loop

Two terminals:

```bash
go run ./cmd/server serve      # :8090 — API, admin UI, migrations, cron
```

```bash
cd web && npm run dev          # :5173 — SPA with HMR, proxies /api and /_/ to :8090
```

Work against **http://localhost:5173**, not `127.0.0.1:5173` — Vite binds the
`localhost` name only. The Go server does not need rebuilding for frontend
changes in this mode, because Vite serves `web/src` directly rather than the
embedded bundle.

## Tests

```bash
CGO_ENABLED=0 go build ./...
go vet ./...
gofmt -l .                     # must print nothing
go test ./...
cd web && npm run build        # type-check + production build
cd web && npm test             # vitest + jsdom
```

`.gitattributes` pins source files to LF, so `gofmt -l .` is meaningful on
Windows. There is no race-detector run: `-race` needs cgo and the build is
deliberately cgo-free.

## WhatsApp

Messaging is the **Meta WhatsApp Cloud API**, server-side. A number is connected
through Meta Embedded Signup on the WhatsApp page — no QR code and no browser
session to keep alive. The access token is stored in a hidden field and never
appears in any API response or realtime payload.

Meta does not call this app directly. All WhatsApp webhooks go through the
Neurocrow webhook router (`hooks.neurocrow.com`), which answers Meta's
verification, checks Meta's signature, and POSTs each event to `/api/webhook`
signed with a shared secret:

- `X-Neurocrow-Timestamp`: unix seconds
- `X-Neurocrow-Signature-256`: `sha256=` + hex HMAC-SHA256(secret, `timestamp + "." + body`)
- `X-Neurocrow-Delivery-Id`: unique per delivery

A POST without a valid signature, or with a timestamp more than 5 minutes off, is
rejected with 401. There is no GET handler. Delivery is at-least-once, so
messages are deduped on the WhatsApp message id. A message more than 15 minutes
old by its own timestamp is stored but gets no automatic reply.

Webhook errors go to PocketBase's logger — the `_logs` collection and the admin
UI — not to stdout.

### What the Cloud API will and will not do

A business can message a customer freely only within 24 hours of that customer's
last message. Outside that window, only a **template approved by Meta** may be
sent. That is why reminders are templates, chosen on the Reminders page from the
account's approved list, and why there is a Templates page for submitting new
ones.

### Phone number format

Country code, digits only, no `+`:

- US: `15512345678`
- Mexico: `5215512345678` — use the `521` prefix for Mexican mobile numbers, not `52`

## Reminders

Each business sets one lead time, a template, a language and a timezone. A cron
job inside `serve` runs every minute, finds the appointments whose reminder has
come due, and sends the template. Sends are deduped by a unique partial index, so
an appointment is reminded about once even if two runs overlap. An appointment
with no client phone number, or whose timezone will not resolve, is skipped
rather than sent wrong.

The cron is registered from the `OnServe` hook, so `migrate` and `seed` never
send.
