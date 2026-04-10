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

## WhatsApp setup

1. Open the app and go to the **WhatsApp Setup** tab
2. Click **Connect WhatsApp**
3. Scan the QR code with your phone (WhatsApp → Settings → Linked Devices → Link a Device)
4. Done — your session is saved and will auto-reconnect on restart

## Phone number format

Numbers must include the country code, digits only:
- US: `15512345678`
- Mexico: `5215512345678` (use `521` prefix for Mexican mobile numbers, not `52`)
