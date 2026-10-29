# Dormant — not part of the application

This directory is the retired QR-code WhatsApp sidecar: an Express + Socket.IO
service wrapping `whatsapp-web.js`, which drove a headless Chrome logged in as a
personal WhatsApp account.

**Nothing in the repository builds it, launches it, imports it or talks to it.**
It has no entry in any build, there is no process that starts it, and no code
left in the Go binary or the SPA references port `:3003`, `socket.io`, or any
file in here. The script that used to start it alongside the old frontend is
gone.

WhatsApp messaging now goes through the **Meta WhatsApp Cloud API**, server-side,
from the Go binary:

- connecting a number — Meta Embedded Signup, `internal/whatsapp`
- sending — `internal/meta`
- receiving — the Neurocrow router POSTs to `/api/webhook`, handled by
  `internal/webhook`

That path needs no browser, no QR scan and no session to keep alive.

## Why it is still here

Two reasons, neither of them "it might come back":

1. **Existing rows.** The sidecar wrote `whatsapp_accounts` rows with
   `account_type = 'web'`, carrying `session_id`, `qr_code` and
   `disconnect_reason`. Those columns are still declared and those rows still
   exist. Every query in the application filters on
   `account_type = 'business_api'` instead of deleting them, so this code is the
   only remaining explanation of what that data is.
2. **The Cloud API is not a superset.** It can only message customers who
   messaged first, or send pre-approved templates. The QR approach had no such
   limit. If that trade-off ever has to be revisited, this is the prior art.

## If you want to delete it

Nothing depends on it, so `rm -rf whatsapp-service/` builds and runs exactly as
before. The only cost is losing the context above.
