#!/bin/bash

# InstantAppoint dev quickstart
# Runs PocketBase, WhatsApp service, and Next.js in parallel
# Ctrl+C kills all three

ROOT="$(cd "$(dirname "$0")" && pwd)"

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${CYAN}Starting InstantAppoint...${NC}"

# Start PocketBase
echo -e "${GREEN}[pocketbase]${NC} Starting on :8090"
pocketbase serve --dir="$ROOT/pb_data" &
PB_PID=$!

# Start WhatsApp service
echo -e "${YELLOW}[whatsapp] ${NC} Starting on :3003"
cd "$ROOT/whatsapp-service" && npm start &
WA_PID=$!

# Start Next.js
echo -e "${GREEN}[next.js]  ${NC} Starting on :3000"
cd "$ROOT" && npm run dev &
NEXT_PID=$!

echo ""
echo -e "${CYAN}All services started. Press Ctrl+C to stop everything.${NC}"
echo ""

# Kill all three on Ctrl+C
trap "echo ''; echo 'Stopping...'; kill $PB_PID $WA_PID $NEXT_PID 2>/dev/null; exit 0" SIGINT SIGTERM

wait
