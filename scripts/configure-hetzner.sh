#!/usr/bin/env bash
# One-time (and safe to re-run) setup of the production PocketBase on the
# Hetzner box, after scripts/deploy-hetzner.sh has the container running:
#
#   1. upserts the admin UI superuser (README "First boot" step 1)
#   2. sets SMTP and the app URL (step 2) - registration sends a verification
#      email whose link is built from the app URL, and the login page refuses
#      unverified accounts, so without this nobody can sign up.
#
# Mail settings live in pb_data, not in the environment, which is why this is
# not part of every deploy: a deploy would otherwise overwrite whatever was
# later changed in the admin UI. Re-run it only to reset those settings.
#
# Reads PB_SUPERUSER_EMAIL, PB_SUPERUSER_PASSWORD, SMTP_USER and SMTP_PASSWORD
# (a Gmail app password) from scripts/.env.prod-secrets.hetzner. Never prints
# them.
set -euo pipefail
cd "$(dirname "$0")/.."

REMOTE=hetzner
REMOTE_DIR=/opt/apps/instantappoint
COMPOSE="docker compose -p instantappoint -f docker-compose.hetzner.yml"
PROD_SECRETS_FILE=scripts/.env.prod-secrets.hetzner
PORT=3012
APP_URL=https://appointments.neurocrow.com

value() { grep -E "^$1=" "$PROD_SECRETS_FILE" | tail -1 | cut -d= -f2-; }

for key in PB_SUPERUSER_EMAIL PB_SUPERUSER_PASSWORD SMTP_USER SMTP_PASSWORD; do
  if [ -z "$(value "$key")" ]; then
    echo "$PROD_SECRETS_FILE is missing $key" >&2
    exit 1
  fi
done

echo "==> Upserting the superuser"
# Credentials go over ssh stdin, not the command line, so they never show up in
# the box's process list or shell history.
printf '%s\n%s\n' "$(value PB_SUPERUSER_EMAIL)" "$(value PB_SUPERUSER_PASSWORD)" |
  ssh "$REMOTE" "cd $REMOTE_DIR && read -r email && read -r password && \
    $COMPOSE exec -T app /app/instantappoint superuser upsert \"\$email\" \"\$password\" --dir=/pb_data"

echo "==> Setting the app URL and SMTP"
SETTINGS=$(python3 - "$APP_URL" "$(value SMTP_USER)" "$(value SMTP_PASSWORD)" <<'EOF'
import json, sys
app_url, user, password = sys.argv[1:4]
print(json.dumps({
    "meta": {
        "appName": "InstantAppoint",
        "appURL": app_url,
        "senderName": "InstantAppoint",
        "senderAddress": user,
    },
    # Gmail on 587 is STARTTLS; PocketBase's "tls" flag means implicit TLS
    # (port 465), so it stays false here and PocketBase upgrades the session.
    "smtp": {
        "enabled": True,
        "host": "smtp.gmail.com",
        "port": 587,
        "username": user,
        "password": password,
        "tls": False,
    },
}))
EOF
)
CREDS=$(python3 -c 'import json,sys; print(json.dumps({"identity": sys.argv[1], "password": sys.argv[2]}))' \
  "$(value PB_SUPERUSER_EMAIL)" "$(value PB_SUPERUSER_PASSWORD)")

# Both bodies travel on stdin, separated by a newline (each is one JSON line).
printf '%s\n%s\n' "$CREDS" "$SETTINGS" | ssh "$REMOTE" "set -e
  read -r creds; read -r settings
  token=\$(printf '%s' \"\$creds\" | curl -sf -H 'Content-Type: application/json' --data-binary @- \
    http://127.0.0.1:$PORT/api/collections/_superusers/auth-with-password | sed -E 's/.*\"token\":\"([^\"]+)\".*/\\1/')
  printf '%s' \"\$settings\" | curl -sf -o /dev/null -w 'PATCH /api/settings: %{http_code}\n' -X PATCH \
    -H \"Authorization: \$token\" -H 'Content-Type: application/json' --data-binary @- \
    http://127.0.0.1:$PORT/api/settings"

echo "==> Done. Admin UI: $APP_URL/_/"
