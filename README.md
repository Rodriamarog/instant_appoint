# InstantAppoint

Appointment scheduling and WhatsApp reminder tool for small service businesses. Manage your calendar and send appointment reminders directly to clients via WhatsApp — no third-party messaging service required.

## What it does

- **Calendar** — create and manage appointments with client name, phone number, time, location, and notes
- **WhatsApp** — link your WhatsApp account by scanning a QR code; the app uses it to send reminders to clients
- **Reminders** — (coming soon) automated reminder messages sent before appointments

Built for solo operators and small businesses where WhatsApp is the primary way to communicate with clients.

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
