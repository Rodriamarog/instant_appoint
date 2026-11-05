# Deploying InstantAppoint — read this first

Production is the **Hetzner box** (`ssh hetzner`, root@5.78.92.14), serving
**https://appointments.neurocrow.com**. Same pattern as neurocrow-hooks and
neurocrow-content: one container built on the box, bound to `127.0.0.1`, public
traffic only through this app's own Cloudflare tunnel.

## Just run it

```bash
./scripts/deploy-hetzner.sh
```

It refuses to run without `scripts/.env.prod-secrets.hetzner` (gitignored). Then it:

1. Runs `go build`, `go vet`, `gofmt -l` and `go test`.
2. Tars the source, clears `cmd internal migrations web` on the box, and extracts it into
   `/opt/apps/instantappoint`. The clearing step stops a file deleted locally from still being built.
3. Writes the box's `.env` with **only** the four runtime variables (`GEMINI_API_KEY`, `META_APP_ID`,
   `META_APP_SECRET`, `NEUROCROW_ROUTER_SECRET`). The secrets file's other keys are for
   `configure-hetzner.sh` only.
4. Builds the image on the box (the `Dockerfile` does the Vite build, then the Go build, then
   distroless with uid 65532) while the old container keeps serving.
5. Runs `up -d --wait`, then checks `127.0.0.1:3012/api/health`.

Schema migrations need no step of their own: `serve` applies the Go migrations in `migrations/`
on boot. Keep them backward-compatible, because the old container keeps serving while the new
image builds.

`META_APP_ID` must equal the id the SPA is built with (`1195277397801905`, the `VITE_META_APP_ID`
default). The script checks this, because a mismatch breaks Embedded Signup.

## What's on the box

| | |
|---|---|
| Code + compose | `/opt/apps/instantappoint` (`docker compose -p instantappoint -f docker-compose.hetzner.yml`) |
| Data | `/opt/apps/instantappoint/data/pb_data` (owned by 65532), mounted at `/pb_data` |
| Port | `127.0.0.1:3012` → container `8090` |
| Tunnel | `instantappoint-hetzner` (`cd0c1fbb-2ea0-4d60-a3f3-a25171e00f59`), unit `cloudflared-instantappoint.service`, config `/etc/cloudflared-instantappoint/config.yml`, creds `/root/.cloudflared-instantappoint/`. Copies of the config and unit are in `deploy/hetzner/cloudflared-instantappoint/` |
| Backups | `scripts/backup-hetzner-db.sh` → `/opt/backups/instantappoint/`, 14 days. They are on the same disk as the data |
| Limits | 256 MB, 0.5 CPU |

The box also runs Monitor Judicial, pedimentos-v2, neurocrow-hooks, neurocrow-content and
neurocrow-site. Never touch their compose projects, tunnels or units. Never add hostnames to
another app's tunnel either.

## First-time setup (already done 2026-11-05)

`./scripts/configure-hetzner.sh` upserts the admin superuser and sets the app URL plus Gmail SMTP
in PocketBase's settings, which live in `pb_data`. It is safe to re-run, but doing so resets SMTP
and app-URL changes made in the admin UI. Admin UI: https://appointments.neurocrow.com/_/

## Tunnel and DNS

DNS was routed with the tunnel's **UUID**, never its name. A tunnel name can silently resolve to
the default config's tunnel; see pedimentosV2's guide.

```bash
cloudflared tunnel --origincert ~/.cloudflared/cert.pem route dns --overwrite-dns \
  cd0c1fbb-2ea0-4d60-a3f3-a25171e00f59 appointments.neurocrow.com
```

Check the output for `tunnelID=cd0c1fbb…`. Then verify functionally: stop the container, and the
public URL should answer 502.

## Webhook router

The neurocrow-hooks destination `instantappoint` (id 1) points at
`https://appointments.neurocrow.com/api/webhook` and is signed with `DEST_SECRET_INSTANTAPPOINT`.
That secret must equal this app's `NEUROCROW_ROUTER_SECRET`. The URL was repointed from the dead
`agendapro.neurocrow.com` on 2026-11-05 (audit row 10). Change it again in the hooks admin UI, not
by editing the database.

## Logs

App warnings, such as a missing env var or webhook errors, go to PocketBase's `_logs`, under
**Logs** in the admin UI. They do not go to `docker logs`.
