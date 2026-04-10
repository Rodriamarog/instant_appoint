import { io, Socket } from 'socket.io-client'
import { pb } from '@/lib/pocketbase'

export class WhatsAppClient {
  private socket: Socket | null = null
  private readonly baseUrl: string
  private userId: string | null = null

  constructor(baseUrl: string = 'http://localhost:3003') {
    this.baseUrl = baseUrl
  }

  // Initialize socket connection
  connect(userId: string) {
    this.userId = userId
    this.socket = io(this.baseUrl, {
      withCredentials: true
    })

    this.socket.on('connect', () => {
      console.log('Connected to WhatsApp service')
      this.socket?.emit('join_user_room', userId)
    })

    this.socket.on('disconnect', () => {
      console.log('Disconnected from WhatsApp service')
    })

    return this.socket
  }

  // Disconnect socket
  disconnect() {
    if (this.socket) {
      this.socket.disconnect()
      this.socket = null
    }
  }

  // Initialize WhatsApp session
  async initSession() {
    try {
      // Get the auth token directly from PocketBase
      const authToken = pb.authStore.token
      console.log('Sending auth token:', !!authToken)

      const response = await fetch('/api/whatsapp/init', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        credentials: 'include'
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Failed to initialize session')
      }

      return await response.json()
    } catch (error) {
      console.error('Error initializing WhatsApp session:', error)
      throw error
    }
  }

  // Get session status
  async getStatus() {
    try {
      const authToken = pb.authStore.token

      const response = await fetch('/api/whatsapp/status', {
        headers: {
          'Authorization': `Bearer ${authToken}`
        },
        credentials: 'include'
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Failed to get status')
      }

      return await response.json()
    } catch (error) {
      console.error('Error getting WhatsApp status:', error)
      throw error
    }
  }

  // Disconnect WhatsApp session
  async disconnectSession() {
    try {
      const authToken = pb.authStore.token

      const response = await fetch('/api/whatsapp/disconnect', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${authToken}`
        },
        credentials: 'include'
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Failed to disconnect')
      }

      return await response.json()
    } catch (error) {
      console.error('Error disconnecting WhatsApp:', error)
      throw error
    }
  }

  // Send test message
  async sendMessage(to: string, message: string) {
    try {
      const authToken = pb.authStore.token

      const response = await fetch('/api/whatsapp/send-message', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        body: JSON.stringify({ to, message }),
        credentials: 'include'
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Failed to send message')
      }

      return await response.json()
    } catch (error) {
      console.error('Error sending WhatsApp message:', error)
      throw error
    }
  }

  // Subscribe to QR code updates
  onQRUpdate(callback: (data: { qrCode: string; status: string }) => void) {
    if (this.socket) {
      this.socket.on('qr_update', callback)
    }
  }

  // Subscribe to connection updates
  onConnectionUpdate(callback: (data: { status: string; connectedNumber?: string; error?: string; reason?: string }) => void) {
    if (this.socket) {
      this.socket.on('connection_update', callback)
    }
  }

  // Remove event listeners
  off(event: string, callback?: (...args: any[]) => void) {
    if (this.socket) {
      this.socket.off(event, callback)
    }
  }
}