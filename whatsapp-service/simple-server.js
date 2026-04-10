require('dotenv').config();
const express = require('express');
const http = require('http');
const socketIo = require('socket.io');
const cors = require('cors');
const SimpleWhatsAppManager = require('./simple-whatsapp');

const app = express();
const server = http.createServer(app);
const io = socketIo(server, {
  cors: {
    origin: process.env.FRONTEND_URL || "http://localhost:3000",
    methods: ["GET", "POST"],
    credentials: true
  }
});

// Middleware
app.use(cors({
  origin: process.env.FRONTEND_URL || "http://localhost:3000",
  credentials: true
}));
app.use(express.json());

// Initialize simplified WhatsApp Manager
const whatsapp = new SimpleWhatsAppManager();
whatsapp.setSocketIO(io);

// Socket.IO connection handling
io.on('connection', (socket) => {
  console.log('Client connected:', socket.id);

  socket.on('join_user_room', (userId) => {
    socket.join(`user_${userId}`);
    console.log(`Socket joined room: user_${userId}`);
  });

  socket.on('disconnect', () => {
    console.log('Client disconnected:', socket.id);
  });
});

// API Routes
app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// Connect WhatsApp
app.post('/api/whatsapp/init-session', async (req, res) => {
  try {
    const { userId } = req.body;
    console.log('=== INIT SESSION REQUEST ===');
    console.log('Raw body:', req.body);
    console.log('User ID received:', userId);
    console.log('User ID type:', typeof userId);
    console.log('===========================');

    if (!userId) {
      return res.status(400).json({ error: 'User ID is required' });
    }

    const result = await whatsapp.connect(userId);
    res.json({ success: true, ...result });
  } catch (error) {
    console.error('Error connecting WhatsApp:', error);
    res.status(500).json({ error: 'Failed to connect WhatsApp', message: error.message });
  }
});

// Get status
app.get('/api/whatsapp/status/:userId', async (req, res) => {
  try {
    const { userId } = req.params;
    const status = await whatsapp.getStatus(userId);
    res.json(status);
  } catch (error) {
    console.error('Error getting status:', error);
    res.status(500).json({ error: 'Failed to get status' });
  }
});

// Disconnect
app.post('/api/whatsapp/disconnect', async (req, res) => {
  try {
    const { userId } = req.body;
    await whatsapp.disconnect(userId);
    res.json({ success: true, message: 'Disconnected successfully' });
  } catch (error) {
    console.error('Error disconnecting:', error);
    res.status(500).json({ error: 'Failed to disconnect' });
  }
});

// Send message
app.post('/api/whatsapp/send-message', async (req, res) => {
  try {
    const { userId, to, message } = req.body;
    const result = await whatsapp.sendMessage(userId, to, message);
    res.json({ success: true, messageId: result.id._serialized });
  } catch (error) {
    console.error('Error sending message:', error);
    res.status(500).json({ error: 'Failed to send message', message: error.message });
  }
});

const PORT = process.env.PORT || 3003;

server.listen(PORT, () => {
  console.log(`Simple WhatsApp service running on port ${PORT}`);
});

// Graceful shutdown
process.on('SIGINT', async () => {
  console.log('Shutting down...');
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
});