const { Client, LocalAuth } = require('whatsapp-web.js');
const QRCode = require('qrcode');
const PocketBase = require('pocketbase/cjs');

class SimpleWhatsAppManager {
  constructor() {
    this.clients = new Map(); // userId -> client
    this.pb = new PocketBase('http://127.0.0.1:8090');
    this.io = null;
  }

  setSocketIO(io) {
    this.io = io;
  }

  async connect(userId) {
    // If a client already exists for this user, check if it's connected
    if (this.clients.has(userId)) {
      const existing = this.clients.get(userId);
      try {
        const state = await existing.getState();
        if (state === 'CONNECTED') {
          return { status: 'connected', message: 'Already connected' };
        }
      } catch (e) {
        // getState() throws when client is still initializing — fall through to replace it
      }
      // Destroy the stale/broken client before creating a new one
      try { await existing.destroy(); } catch (e) {}
      this.clients.delete(userId);
    }

    console.log(`Starting WhatsApp connection for user: ${userId}`);

    const client = new Client({
      authStrategy: new LocalAuth({ clientId: userId }),
      puppeteer: {
        headless: false,
        args: ['--no-sandbox', '--disable-setuid-sandbox']
      }
    });

    // Store client immediately so getStatus() returns 'initializing'
    this.clients.set(userId, client);

    client.on('qr', async (qr) => {
      console.log(`QR generated for ${userId}`);
      const qrCode = await QRCode.toDataURL(qr);

      await this.updateDatabase(userId, 'waiting_qr', null, qrCode);

      if (this.io) {
        this.io.to(`user_${userId}`).emit('qr_update', { qrCode, status: 'waiting_qr' });
      }
    });

    client.on('ready', async () => {
      console.log(`WhatsApp ready for ${userId}`);
      const phoneNumber = client.info?.wid?.user || 'unknown';

      await this.updateDatabase(userId, 'connected', phoneNumber, '');

      if (this.io) {
        this.io.to(`user_${userId}`).emit('connection_update', {
          status: 'connected',
          connectedNumber: phoneNumber
        });
      }
    });

    client.on('disconnected', async (reason) => {
      console.log(`WhatsApp disconnected for ${userId} — reason: ${reason}`);

      await this.updateDatabase(userId, 'disconnected', null, null, reason);

      if (this.io) {
        this.io.to(`user_${userId}`).emit('connection_update', { status: 'disconnected', reason });
      }

      this.clients.delete(userId);
    });

    client.on('auth_failure', async (msg) => {
      const reason = `auth_failure: ${msg}`;
      console.log(`WhatsApp auth failure for ${userId} — reason: ${reason}`);

      await this.updateDatabase(userId, 'disconnected', null, null, reason);

      if (this.io) {
        this.io.to(`user_${userId}`).emit('connection_update', { status: 'disconnected', reason });
      }

      this.clients.delete(userId);
    });

    // Fire-and-forget: initialize() resolves before QR is emitted, so we must not await it
    client.initialize().catch((err) => {
      console.error(`Initialize failed for ${userId}:`, err.message);
      this.clients.delete(userId);
    });

    return { status: 'initializing' };
  }

  async updateDatabase(userId, status, phoneNumber = null, qrCode = null, disconnectReason = null) {
    try {
      let record = null;
      try {
        record = await this.pb.collection('whatsapp_accounts')
          .getFirstListItem(`user_id = "${userId}"`);
      } catch (e) {
        // Record doesn't exist yet — will create below
      }

      const data = {
        user_id: userId,
        session_id: userId,
        status,
        is_active: status === 'connected',
        ...(phoneNumber !== null && { phone_number: phoneNumber }),
        ...(qrCode !== null && { qr_code: qrCode }),
        ...(disconnectReason !== null && { disconnect_reason: disconnectReason }),
      };

      if (record) {
        await this.pb.collection('whatsapp_accounts').update(record.id, data);
      } else {
        await this.pb.collection('whatsapp_accounts').create(data);
      }

      console.log(`DB updated: ${userId} -> ${status}`);
    } catch (error) {
      // DB failures must never crash the WhatsApp flow
      console.error(`DB update failed (non-fatal): ${error.message}`);
    }
  }

  async getStatus(userId) {
    const client = this.clients.get(userId);
    if (!client) {
      return { status: 'not_initialized' };
    }

    try {
      const state = await client.getState();
      return {
        status: state === 'CONNECTED' ? 'connected' : 'waiting_qr',
        connectedNumber: client.info?.wid?.user
      };
    } catch (error) {
      // getState() throws when client is still starting up
      return { status: 'initializing' };
    }
  }

  async disconnect(userId) {
    const client = this.clients.get(userId);
    if (client) {
      await client.destroy();
      this.clients.delete(userId);
      await this.updateDatabase(userId, 'disconnected');
    }
  }

  startScheduler() {
    console.log('Reminder scheduler started (checking every 60s)');
    this.runReminderCheck();
    setInterval(() => this.runReminderCheck(), 60_000);
  }

  async runReminderCheck() {
    const now = new Date();
    // Look 1h into the past so recently-passed appointments aren't missed if the
    // service was briefly down or the scheduler just started.
    const windowStart = new Date(now.getTime() - 60 * 60 * 1000);
    const windowEnd = new Date(now.getTime() + 25 * 60 * 60 * 1000);

    let events;
    try {
      events = await this.pb.collection('calendar_events').getFullList({
        filter: `start_time >= "${windowStart.toISOString()}" && start_time <= "${windowEnd.toISOString()}"`,
      });
    } catch (err) {
      console.error('Scheduler: failed to query events:', err.message);
      return;
    }

    for (const event of events) {
      try {
        if (!event.client_phone) continue;

        // Get this user's reminder settings (fall back to defaults)
        let timingMinutes = 1440;
        let isActive = true;
        try {
          const settings = await this.pb.collection('reminder_settings')
            .getFirstListItem(`user_id = "${event.user_id}"`);
          timingMinutes = settings.timing_minutes;
          isActive = settings.is_active;
        } catch { /* no settings record — use defaults */ }

        if (!isActive) continue;

        // Check if reminder time has arrived
        const reminderTime = new Date(new Date(event.start_time).getTime() - timingMinutes * 60 * 1000);
        if (reminderTime > now) continue;

        // Deduplication: skip if already sent
        try {
          await this.pb.collection('whatsapp_messages')
            .getFirstListItem(`event_id = "${event.id}" && message_type = "reminder"`);
          continue; // found → already sent
        } catch { /* not found → proceed */ }

        // Check WhatsApp client is connected
        const client = this.clients.get(event.user_id);
        if (!client) {
          console.log(`Scheduler: no WhatsApp client for user ${event.user_id}, skipping event ${event.id}`);
          continue;
        }
        let state;
        try { state = await client.getState(); } catch { continue; }
        if (state !== 'CONNECTED') continue;

        // Send the message
        const chatId = `${event.client_phone.replace(/\D/g, '')}@c.us`;
        const message = event.notes || 'Reminder: you have an appointment soon.';
        let msgId = '';
        let status = 'failed';
        try {
          const sent = await client.sendMessage(chatId, message);
          msgId = sent.id._serialized;
          status = 'sent';
          console.log(`Scheduler: reminder sent for event ${event.id} to ${event.client_phone}`);
        } catch (err) {
          console.error(`Scheduler: failed to send reminder for event ${event.id}:`, err.message);
        }

        // Log result (even on failure, so we know it was attempted)
        await this.pb.collection('whatsapp_messages').create({
          user_id: event.user_id,
          event_id: event.id,
          to_number: event.client_phone,
          message_content: message,
          message_type: 'reminder',
          status,
          sent_at: new Date().toISOString(),
          message_id: msgId,
        }).catch(err => console.error('Scheduler: failed to log message:', err.message));

      } catch (err) {
        console.error(`Scheduler: unexpected error processing event ${event.id}:`, err.message);
      }
    }
  }

  async reconnectActiveSessions() {
    console.log('Checking for active WhatsApp sessions to restore...');
    try {
      const result = await this.pb.collection('whatsapp_accounts').getList(1, 100, {
        filter: 'is_active = true'
      });

      if (result.items.length === 0) {
        console.log('No active sessions to restore.');
        return;
      }

      console.log(`Restoring ${result.items.length} WhatsApp session(s)...`);

      for (let i = 0; i < result.items.length; i++) {
        const account = result.items[i];
        // Stagger reconnects by 3s each to avoid spawning all Puppeteer instances at once
        setTimeout(() => {
          console.log(`Auto-reconnecting user: ${account.user_id}`);
          this.connect(account.user_id).catch((err) => {
            console.error(`Auto-reconnect failed for ${account.user_id}:`, err.message);
          });
        }, i * 3000);
      }
    } catch (error) {
      console.error('Failed to query active sessions (non-fatal):', error.message);
    }
  }

  async sendMessage(userId, to, message) {
    const client = this.clients.get(userId);
    if (!client) {
      throw new Error('Not connected');
    }

    const chatId = to.includes('@') ? to : `${to.replace(/\D/g, '')}@c.us`;
    return await client.sendMessage(chatId, message);
  }
}

module.exports = SimpleWhatsAppManager;
