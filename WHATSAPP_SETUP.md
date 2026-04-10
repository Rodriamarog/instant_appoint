# WhatsApp Integration Setup Guide

This guide explains how to set up and use the WhatsApp integration for InstantAppoint.

## Architecture Overview

The WhatsApp integration consists of:

1. **Standalone WhatsApp Service** (`/whatsapp-service/`) - Handles WhatsApp Web.js clients
2. **Next.js API Routes** (`/app/api/whatsapp/`) - Bridge between frontend and WhatsApp service
3. **Real-time Communication** - Socket.IO for QR code updates and status changes
4. **PocketBase Collections** - Store WhatsApp account data and message logs

## Required PocketBase Collections

Before using the WhatsApp integration, create these collections in PocketBase:

### 1. whatsapp_accounts
```javascript
{
  user_id: "relation to users",
  phone_number: "text",
  status: "text", // disconnected|connecting|connected|error
  qr_code: "text", // temporary QR code data
  session_id: "text", // unique session identifier
  connected_at: "datetime",
  last_seen: "datetime",
  disconnected_at: "datetime",
  device_name: "text",
  is_active: "boolean"
}
```

**Access Rules** (all rules):
```javascript
user_id = @request.auth.id
```

### 2. whatsapp_messages
```javascript
{
  user_id: "relation to users",
  whatsapp_account_id: "relation to whatsapp_accounts",
  to_number: "text",
  message_content: "text",
  message_type: "text", // text|media
  status: "text", // pending|sent|delivered|failed
  sent_at: "datetime",
  message_id: "text", // WhatsApp message ID
  appointment_id: "text" // optional relation to appointments
}
```

**Access Rules** (all rules):
```javascript
user_id = @request.auth.id
```

## Setup Instructions

### 1. Start the WhatsApp Service

First terminal:
```bash
cd whatsapp-service
npm start
```

The service will run on port 3001 and handle WhatsApp Web.js connections.

### 2. Start the Next.js App

Second terminal:
```bash
npm run dev
```

The main app runs on port 3000.

### 3. Start PocketBase

Third terminal:
```bash
# Start your PocketBase instance on port 8090
```

## How to Connect WhatsApp

1. **Login to InstantAppoint** - Go to http://localhost:3000 and sign in
2. **Navigate to WhatsApp Tab** - Click on the "WhatsApp Setup" tab in the dashboard
3. **Connect WhatsApp** - Click "Connect WhatsApp" button
4. **Scan QR Code** - A QR code will appear. Open WhatsApp on your phone:
   - Go to Settings → Linked Devices → Link a Device
   - Scan the QR code displayed on the screen
5. **Connected!** - Once scanned, the status will change to "Connected" and show your phone number

## Features

### Real-time Updates
- QR codes update automatically every 30 seconds
- Connection status updates in real-time via WebSocket
- No need to refresh the page

### Multi-user Support
- Each user has their own isolated WhatsApp session
- Sessions are persistent (no need to re-scan after server restart)
- User data is completely isolated in the database

### Test Messaging
- Send test messages once connected
- Enter phone number with country code (e.g., 1234567890)
- Messages are logged in the database

### Session Management
- Automatic session cleanup for inactive connections
- Graceful disconnection and reconnection
- Error handling for failed connections

## API Endpoints

The following API endpoints are available:

- `POST /api/whatsapp/init` - Initialize WhatsApp session
- `GET /api/whatsapp/status` - Get connection status
- `POST /api/whatsapp/disconnect` - Disconnect WhatsApp session
- `POST /api/whatsapp/send-message` - Send WhatsApp message

All endpoints require authentication via PocketBase session cookies.

## Troubleshooting

### "Session not found" error
- Make sure both WhatsApp service and Next.js app are running
- Check that PocketBase collections are created with correct access rules
- Try refreshing the page and reconnecting

### QR code not appearing
- Check browser console for errors
- Ensure WhatsApp service is running on port 3001
- Try clicking "Connect WhatsApp" again

### Messages not sending
- Verify WhatsApp is connected (green "Connected" badge)
- Include country code in phone number (no + sign needed)
- Check that the recipient number is valid

### Session disconnected
- This is normal - WhatsApp Web sessions can timeout
- Simply click "Connect WhatsApp" and scan the QR code again
- Sessions are automatically cleaned up after 30 minutes of inactivity

## Security Notes

- Each user has completely isolated WhatsApp sessions
- QR codes are temporary and user-specific
- All API endpoints require valid PocketBase authentication
- Message logs are stored per user with proper access controls

## Production Deployment

For production deployment:

1. Update `WHATSAPP_SERVICE_URL` in `.env` to point to your production WhatsApp service
2. Configure proper CORS settings in the WhatsApp service
3. Use process manager (PM2) for the WhatsApp service
4. Ensure proper firewall rules for ports 3000 and 3001
5. Consider using Redis for session storage in clustered environments