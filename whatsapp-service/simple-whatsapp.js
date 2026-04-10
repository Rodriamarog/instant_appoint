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
        headless: true,
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

    client.on('disconnected', async () => {
      console.log(`WhatsApp disconnected for ${userId}`);

      await this.updateDatabase(userId, 'disconnected');

      if (this.io) {
        this.io.to(`user_${userId}`).emit('connection_update', { status: 'disconnected' });
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

  async updateDatabase(userId, status, phoneNumber = null, qrCode = null) {
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
