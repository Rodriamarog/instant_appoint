'use client'

import { useState, useEffect } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Loader2 } from 'lucide-react'
import { pb } from '@/lib/pocketbase'
import { WhatsAppClient } from '@/lib/whatsapp-client'
import WhatsAppEmbeddedSignup from '@/components/whatsapp/embedded-signup'

const COUNTRY_CODES = [
  { label: '+1',  value: '1',   flag: '🇺🇸' },
  { label: '+52', value: '521', flag: '🇲🇽' },
]

export default function WhatsAppPage() {
  const [whatsappClient] = useState(() => new WhatsAppClient())
  const [whatsappStatus, setWhatsappStatus] = useState('not_initialized')
  const [accountType, setAccountType] = useState<string | null>(null)
  const [disconnectReason, setDisconnectReason] = useState<string | null>(null)
  const [qrCode, setQrCode] = useState<string | null>(null)
  const [connectedNumber, setConnectedNumber] = useState<string | null>(null)
  const [isConnecting, setIsConnecting] = useState(false)
  const [testMessage, setTestMessage] = useState('')
  const [testNumber, setTestNumber] = useState('')
  const [testCountryCode, setTestCountryCode] = useState('1')
  const [isSendingTest, setIsSendingTest] = useState(false)

  useEffect(() => {
    const userId = pb.authStore.model?.id
    if (!userId) return

    const socket = whatsappClient.connect(userId)
    void socket

    whatsappClient.onQRUpdate((data) => {
      setQrCode(data.qrCode)
      setWhatsappStatus(data.status)
    })

    whatsappClient.onConnectionUpdate((data) => {
      setWhatsappStatus(data.status)
      if (data.connectedNumber) {
        setConnectedNumber(data.connectedNumber)
        setQrCode(null)
      }
      if (data.status === 'disconnected') {
        setConnectedNumber(null)
        setQrCode(null)
        setDisconnectReason(data.reason ?? null)
      }
      setIsConnecting(false)
    })

    loadWhatsAppStatus()

    return () => { whatsappClient.disconnect() }
  }, [])

  const loadWhatsAppStatus = async () => {
    try {
      const status = await whatsappClient.getStatus()
      setWhatsappStatus(status.status)
      setConnectedNumber(status.connectedNumber ?? null)
      setAccountType(status.accountType ?? null)
    } catch {
      setWhatsappStatus('not_initialized')
    }
  }

  const startWhatsAppConnection = async () => {
    setIsConnecting(true)
    setQrCode(null)
    try {
      await whatsappClient.initSession()
      setWhatsappStatus('waiting_qr')
    } catch {
      setWhatsappStatus('error')
    } finally {
      setIsConnecting(false)
    }
  }

  const disconnectWhatsApp = async () => {
    try {
      await whatsappClient.disconnectSession()
      setWhatsappStatus('disconnected')
      setConnectedNumber(null)
      setQrCode(null)
    } catch {
      console.error('Error disconnecting WhatsApp')
    }
  }

  const sendTestMessage = async () => {
    if (!testNumber || !testMessage) return
    setIsSendingTest(true)
    try {
      await whatsappClient.sendMessage(`${testCountryCode}${testNumber}`, testMessage)
      setTestMessage('')
      alert('Test message sent successfully!')
    } catch {
      alert('Failed to send test message')
    } finally {
      setIsSendingTest(false)
    }
  }

  return (
    <div className="space-y-5">
      <div>
        <h2 className="text-2xl font-bold text-gray-900 mb-2">WhatsApp Integration</h2>
        <p className="text-gray-600">
          Connect your WhatsApp account to send appointment notifications.
        </p>
      </div>

      {/* WhatsApp Business API */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>WhatsApp Business API</CardTitle>
            <Badge variant={whatsappStatus === 'connected' && accountType === 'business_api' ? 'default' : 'outline'}>
              {whatsappStatus === 'connected' && accountType === 'business_api' ? 'Connected' : 'Not connected'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {whatsappStatus === 'connected' && accountType === 'business_api' ? (
            <>
              <div className="p-3 bg-green-50 border border-green-200 rounded-lg">
                <p className="text-sm text-green-800">
                  <strong>Connected number:</strong> +{connectedNumber ?? '—'}
                </p>
              </div>
              <p className="text-sm text-gray-500">
                Your WhatsApp Business number is connected via the Cloud API. Messages will be handled automatically.
              </p>
              <WhatsAppEmbeddedSignup onSuccess={loadWhatsAppStatus} />
            </>
          ) : (
            <>
              <p className="text-sm text-gray-600">
                Connect your WhatsApp Business number via Meta&apos;s official API.
              </p>
              <WhatsAppEmbeddedSignup onSuccess={loadWhatsAppStatus} />
            </>
          )}
        </CardContent>
      </Card>

      {/* Test Message */}
      {whatsappStatus === 'connected' && (
        <Card>
          <CardHeader>
            <CardTitle>Send Test Message</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-3">
              <div>
                <label htmlFor="testNumber" className="block text-sm font-medium text-gray-700 mb-1">
                  Phone Number
                </label>
                <div className="flex gap-2">
                  <select
                    value={testCountryCode}
                    onChange={(e) => setTestCountryCode(e.target.value)}
                    className="border border-gray-300 rounded-md px-2 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-black"
                  >
                    {COUNTRY_CODES.map((c) => (
                      <option key={c.value} value={c.value}>{c.flag} {c.label}</option>
                    ))}
                  </select>
                  <Input
                    id="testNumber"
                    type="text"
                    placeholder="5512345678"
                    value={testNumber}
                    onChange={(e) => setTestNumber(e.target.value)}
                    className="flex-1"
                  />
                </div>
              </div>
              <div>
                <label htmlFor="testMessage" className="block text-sm font-medium text-gray-700 mb-1">
                  Message
                </label>
                <Input
                  id="testMessage"
                  type="text"
                  placeholder="Test message from InstantAppoint"
                  value={testMessage}
                  onChange={(e) => setTestMessage(e.target.value)}
                />
              </div>
              <Button
                onClick={sendTestMessage}
                disabled={isSendingTest || !testNumber || !testMessage}
                className="w-full bg-black text-white hover:bg-gray-800"
              >
                {isSendingTest ? 'Sending...' : 'Send Test Message'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
