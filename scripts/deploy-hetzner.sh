#!/usr/bin/env bash
# Deploys InstantAppoint to the Hetzner box (root@5.78.92.14, ssh alias
# `hetzner`) as one Docker container via docker-compose.hetzner.yml. Copied from
# neurocrow-hooks' scripts/deploy-hetzner.sh: plain ssh + tar transport, builds
# on the server, no registry.
#
# Public traffic for appointments.neurocrow.com comes in through this app's own
# tunnel (instantappoint-hetzner, systemd unit cloudflared-instantappoint.service,
# /etc/cloudflared-instantappoint/config.yml; copies in
# deploy/hetzner/cloudflared-instantappoint/).
#
# This box also runs Monitor Judicial's containers (whatsapp-bot,
# tribunal-scraper, rag-api-server, legal-rag-*), pedimentos-v2,
# neurocrow-hooks, neurocrow-content and neurocrow-site. This script only
# touches /opt/apps/instantappoint and the `instantappoint` compose project. It
# never touches a tunnel, systemd unit or other compose project.
#
# Schema migrations need no step here: `serve` applies pending Go migrations on
# boot (README "Run").
set -euo pipefail
cd "$(dirname "$0")/.."

REMOTE=hetzner
REMOTE_DIR=/opt/apps/instantappoint
COMPOSE="docker compose -p instantappoint -f docker-compose.hetzner.yml"
PROD_SECRETS_FILE=scripts/.env.prod-secrets.hetzner
PORT=3012

# The app's whole runtime environment (README "Environment variables"). Only
# these go into the container's .env; the secrets file also holds the
# superuser and SMTP credentials that scripts/configure-hetzner.sh uses, which
# the running app has no business seeing.
APP_KEYS=(GEMINI_API_KEY META_APP_ID META_APP_SECRET NEUROCROW_ROUTER_SECRET)

if [ ! -f "$PROD_SECRETS_FILE" ]; then
  echo "Missing $PROD_SECRETS_FILE - it holds the prod env (Gemini key, Meta app id and" >&2
  echo "secret, router secret). Refusing to deploy without it." >&2
  exit 1
fi
missing=()
for key in "${APP_KEYS[@]}"; do
  grep -qE "^${key}=.+" "$PROD_SECRETS_FILE" || missing+=("$key")
done
if [ "${#missing[@]}" -gt 0 ]; then
  echo "$PROD_SECRETS_FILE is missing: ${missing[*]}" >&2
  exit 1
fi
# The browser bundle bakes in the Meta app id at build time (VITE_META_APP_ID,
# default in web/src). If the server's differs, Embedded Signup codes are
# refused by Meta, so catch it before shipping.
BUNDLE_APP_ID=1195277397801905
if ! grep -qE "^META_APP_ID=${BUNDLE_APP_ID}\$" "$PROD_SECRETS_FILE"; then
  echo "META_APP_ID in $PROD_SECRETS_FILE is not ${BUNDLE_APP_ID}, the app id the SPA is built" >&2
  echo "with. Set VITE_META_APP_ID/VITE_META_CONFIG_ID in the Dockerfile's web stage, or fix the id." >&2
  exit 1
fi

echo "==> Running checks"
CGO_ENABLED=0 go build ./...
go vet ./...
if [ -n "$(gofmt -l .)" ]; then
  echo "gofmt -l reports unformatted files:" >&2
  gofmt -l . >&2
  exit 1
fi
go test ./...

TARBALL=$(mktemp /tmp/instantappoint-deploy-XXXXXX.tar.gz)
cleanup() { rm -f "$TARBALL"; }
trap cleanup EXIT

echo "==> Packaging source"
tar czf "$TARBALL" \
  --exclude=.git \
  --exclude=./.claude \
  --exclude=./data \
  --exclude=./pb_data \
  --exclude=./instantappoint \
  --exclude=node_modules \
  --exclude=./web/dist \
  --exclude=./whatsapp-service \
  --exclude=.env \
  --exclude='.env.*' \
  --exclude='scripts/.env.prod-secrets*' \
  .

echo "==> Streaming source to $REMOTE:$REMOTE_DIR"
# Extracting over the previous copy never removes anything, so a file deleted
# locally would linger and still be built (this bit pedimentos-v2). Clear the
# source dirs first; data/ and .env are left alone.
ssh "$REMOTE" "mkdir -p $REMOTE_DIR && cd $REMOTE_DIR && rm -rf cmd internal migrations web && tar xzf - -C $REMOTE_DIR" < "$TARBALL"

echo "==> Copying env file (${APP_KEYS[*]})"
keys_re="^($(IFS='|'; echo "${APP_KEYS[*]}"))="
grep -E "$keys_re" "$PROD_SECRETS_FILE" | ssh "$REMOTE" "umask 077 && cat > $REMOTE_DIR/.env"

echo "==> Preparing the data dir (owned by the image's uid 65532)"
ssh "$REMOTE" "mkdir -p $REMOTE_DIR/data/pb_data && chown 65532:65532 $REMOTE_DIR/data $REMOTE_DIR/data/pb_data && chmod 700 $REMOTE_DIR/data"

echo "==> Building the image (the running container is not replaced yet)"
ssh "$REMOTE" "cd $REMOTE_DIR && $COMPOSE build app 2>&1" | tail -20

echo "==> Starting the new container"
ssh "$REMOTE" "cd $REMOTE_DIR && $COMPOSE up -d --wait app 2>&1" | tail -10

echo "==> Container on $REMOTE:"
ssh "$REMOTE" "cd $REMOTE_DIR && $COMPOSE ps && curl -s -o /dev/null -w 'local /api/health: %{http_code}\n' http://127.0.0.1:$PORT/api/health"
