'use client'

import { useState, useEffect } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Loader2 } from 'lucide-react'
import { pb } from '@/lib/pocketbase'
import { WhatsAppClient } from '@/lib/whatsapp-client'
import WhatsAppEmbeddedSignup from '@/components/whatsapp/embedded-signup'

interface CloudAccount {
  id: string
  phone_number: string
  phone_number_id: string
  waba_id: string
  verified_name: string
}

export default function WhatsAppPage() {
  const [whatsappClient] = useState(() => new WhatsAppClient())

  // Legacy QR state
  const [legacyStatus, setLegacyStatus] = useState('not_initialized')
  const [legacyNumber, setLegacyNumber] = useState<string | null>(null)
  const [disconnectReason, setDisconnectReason] = useState<string | null>(null)
  const [qrCode, setQrCode] = useState<string | null>(null)
  const [isConnecting, setIsConnecting] = useState(false)

  // Cloud API state
  const [cloudAccounts, setCloudAccounts] = useState<CloudAccount[]>([])

  useEffect(() => {
    const userId = pb.authStore.model?.id
    if (!userId) return

    const socket = whatsappClient.connect(userId)
    void socket

    whatsappClient.onQRUpdate((data) => {
      setQrCode(data.qrCode)
      setLegacyStatus(data.status)
    })

    whatsappClient.onConnectionUpdate((data) => {
      setLegacyStatus(data.status)
      if (data.connectedNumber) {
        setLegacyNumber(data.connectedNumber)
        setQrCode(null)
      }
      if (data.status === 'disconnected') {
        setLegacyNumber(null)
        setQrCode(null)
        setDisconnectReason(data.reason ?? null)
      }
      setIsConnecting(false)
    })

    loadStatus()

    return () => { whatsappClient.disconnect() }
  }, [])

  const loadStatus = async () => {
    try {
      const res = await fetch('/api/whatsapp/status', {
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
      })
      if (!res.ok) return
      const data = await res.json()
      setLegacyStatus(data.legacy?.status ?? 'not_initialized')
      setLegacyNumber(data.legacy?.connectedNumber ?? null)
      setCloudAccounts(data.cloudAccounts ?? [])
    } catch {
      setLegacyStatus('not_initialized')
    }
  }

  const startLegacyConnection = async () => {
    setIsConnecting(true)
    setQrCode(null)
    try {
      await whatsappClient.initSession()
      setLegacyStatus('waiting_qr')
    } catch {
      setLegacyStatus('error')
    } finally {
      setIsConnecting(false)
    }
  }

  const disconnectLegacy = async () => {
    try {
      await whatsappClient.disconnectSession()
      setLegacyStatus('disconnected')
      setLegacyNumber(null)
      setQrCode(null)
    } catch {
      console.error('Error disconnecting WhatsApp')
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

      {/* WhatsApp Business Cloud API */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>WhatsApp Business API</CardTitle>
            <Badge variant={cloudAccounts.length > 0 ? 'default' : 'outline'}>
              {cloudAccounts.length > 0 ? 'Connected' : 'Not connected'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {cloudAccounts.length > 0 && (
            <div className="space-y-2">
              {cloudAccounts.map(acc => (
                <div key={acc.id} className="p-3 bg-green-50 border border-green-200 rounded-lg">
                  <p className="text-sm text-green-800">
                    <strong>{acc.verified_name || 'WhatsApp Business'}</strong>
                  </p>
                  <p className="text-sm text-green-700 mt-0.5">+{acc.phone_number || acc.phone_number_id}</p>
                  <p className="text-xs text-green-500 mt-0.5">WABA: {acc.waba_id}</p>
                </div>
              ))}
            </div>
          )}
          <p className="text-sm text-gray-600">
            {cloudAccounts.length > 0
              ? 'Your number is connected via the official Cloud API. Connect another number below.'
              : "Connect your WhatsApp Business number via Meta's official API."}
          </p>
          <WhatsAppEmbeddedSignup onSuccess={loadStatus} />
        </CardContent>
      </Card>

      {/* Legacy QR Connection */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>WhatsApp Web (QR)</CardTitle>
            <Badge variant={
              legacyStatus === 'connected' ? 'default' :
              legacyStatus === 'waiting_qr' || legacyStatus === 'initializing' ? 'secondary' :
              legacyStatus === 'error' ? 'destructive' : 'outline'
            }>
              {legacyStatus === 'connected' && 'Connected'}
              {legacyStatus === 'waiting_qr' && 'Waiting for QR scan'}
              {legacyStatus === 'initializing' && (
                <span className="flex items-center gap-1">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Generating QR...
                </span>
              )}
              {(legacyStatus === 'disconnected' || legacyStatus === 'not_initialized') && 'Not connected'}
              {legacyStatus === 'error' && 'Error'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {legacyNumber && (
            <div className="p-3 bg-green-50 border border-green-200 rounded-lg">
              <p className="text-sm text-green-800">
                <strong>Connected number:</strong> +{legacyNumber}
              </p>
            </div>
          )}
          {legacyStatus === 'disconnected' && disconnectReason && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-lg">
              <p className="text-sm text-red-800"><strong>Disconnected:</strong> {disconnectReason}</p>
            </div>
          )}
          {(qrCode || legacyStatus === 'initializing' || legacyStatus === 'waiting_qr') && (
            <div className="text-center space-y-3">
              <p className="text-sm text-gray-600">
                Open WhatsApp → Settings → Linked Devices → Link a Device
              </p>
              <div className="flex justify-center">
                {qrCode ? (
                  <img src={qrCode} alt="WhatsApp QR Code" className="w-64 h-64 border border-gray-200 rounded-lg" />
                ) : (
                  <div className="w-64 h-64 border border-gray-200 rounded-lg flex items-center justify-center bg-gray-50">
                    <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
                  </div>
                )}
              </div>
            </div>
          )}
          <div className="flex gap-3">
            {legacyStatus === 'connected' ? (
              <Button onClick={disconnectLegacy} className="px-4 py-2 text-sm bg-black text-white hover:bg-gray-800">
                Disconnect
              </Button>
            ) : (
              <Button onClick={startLegacyConnection} disabled={isConnecting} className="px-4 py-2 text-sm bg-black text-white hover:bg-gray-800">
                {isConnecting ? 'Starting...' : legacyStatus === 'waiting_qr' ? 'Restart' : 'Connect via QR'}
              </Button>
            )}
            <Button onClick={loadStatus} variant="outline" className="px-4 py-2 text-sm">
              Refresh
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
